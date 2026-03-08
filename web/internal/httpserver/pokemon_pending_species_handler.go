package httpserver

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/victoroliveirab/vibe-pokemongo-appraisal-app/web/internal/upload"
)

type pokemonPendingSpeciesHandler struct {
	store upload.Store
}

func newPokemonPendingSpeciesHandler(store upload.Store) http.Handler {
	return &pokemonPendingSpeciesHandler{store: store}
}

func (h *pokemonPendingSpeciesHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	sess, ok := SessionFromContext(r.Context())
	if !ok {
		writeAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error", nil)
		return
	}

	readings, err := h.store.ListPendingReadingsBySession(r.Context(), sess.ID)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error", nil)
		return
	}

	payloadReadings := make([]pokemonPendingReadingResponse, 0, len(readings))
	for _, reading := range readings {
		payloadOptions := make([]pokemonPendingOptionResponse, 0, len(reading.Options))
		for _, option := range reading.Options {
			payloadOptions = append(payloadOptions, pokemonPendingOptionResponse{
				ID:            option.ID,
				SpeciesName:   option.SpeciesName,
				MatchMode:     option.MatchMode,
				MatchDistance: option.MatchDistance,
				OptionRank:    option.OptionRank,
			})
		}

		payloadReadings = append(payloadReadings, pokemonPendingReadingResponse{
			ID:         reading.ID,
			JobID:      reading.JobID,
			UploadID:   reading.UploadID,
			CP:         reading.CP,
			HP:         reading.HP,
			IVs:        pokemonResultIVsResponse{Attack: reading.IVAttack, Defense: reading.IVDefense, Stamina: reading.IVStamina},
			Level:      pokemonResultLevelResponse{Estimate: reading.LevelEstimate, Confidence: reading.LevelConfidence, Method: reading.LevelMethod},
			Source:     pokemonPendingReadingSourceResponse{Type: reading.SourceType, FrameTimestampMS: reading.FrameTimestampMS},
			Confidence: reading.ExtractionConfidence,
			Status:     reading.Status,
			CreatedAt:  reading.CreatedAt.Format(time.RFC3339Nano),
			Options:    payloadOptions,
		})
	}

	writeJSON(w, http.StatusOK, pokemonPendingSpeciesEnvelopeResponse{Readings: payloadReadings})
}

type pokemonPendingSpeciesResolveHandler struct {
	store upload.Store
	now   func() time.Time
}

func newPokemonPendingSpeciesResolveHandler(store upload.Store, nowFn func() time.Time) http.Handler {
	return &pokemonPendingSpeciesResolveHandler{
		store: store,
		now:   nowFn,
	}
}

type resolvePendingReadingRequest struct {
	OptionID string `json:"optionId"`
}

func (h *pokemonPendingSpeciesResolveHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		w.Header().Set("Allow", http.MethodPatch)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	sess, ok := SessionFromContext(r.Context())
	if !ok {
		writeAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error", nil)
		return
	}

	var payload resolvePendingReadingRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeAPIError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request payload", nil)
		return
	}
	optionID := strings.TrimSpace(payload.OptionID)
	if optionID == "" {
		writeAPIError(w, http.StatusBadRequest, "MISSING_OPTION_ID", "optionId is required", nil)
		return
	}

	resolvedResult, err := h.store.ResolvePendingReading(r.Context(), upload.ResolvePendingReadingParams{
		ReadingID: r.PathValue("readingId"),
		OptionID:  optionID,
		SessionID: sess.ID,
		Now:       h.now(),
	})
	if err != nil {
		if errors.Is(err, upload.ErrPendingReadingNotFound) {
			writeAPIError(w, http.StatusNotFound, "READING_NOT_FOUND", "Pending reading not found", nil)
			return
		}
		if errors.Is(err, upload.ErrPendingReadingLocked) {
			writeAPIError(w, http.StatusConflict, "READING_LOCKED", "Pending reading already resolved", nil)
			return
		}
		if errors.Is(err, upload.ErrPendingOptionNotFound) {
			writeAPIError(w, http.StatusNotFound, "OPTION_NOT_FOUND", "Pending species option not found", nil)
			return
		}

		writeAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error", nil)
		return
	}

	writeJSON(w, http.StatusOK, pokemonPendingSpeciesResolveResponse{
		Result: pokemonResultResponse{
			ID:                  resolvedResult.ID,
			SpeciesName:         resolvedResult.SpeciesName,
			CP:                  resolvedResult.CP,
			HP:                  resolvedResult.HP,
			PowerUpStardustCost: resolvedResult.PowerUpStardustCost,
			IVs: pokemonResultIVsResponse{
				Attack:  resolvedResult.IVAttack,
				Defense: resolvedResult.IVDefense,
				Stamina: resolvedResult.IVStamina,
			},
			Level: pokemonResultLevelResponse{
				Estimate:   resolvedResult.LevelEstimate,
				Confidence: resolvedResult.LevelConfidence,
				Method:     resolvedResult.LevelMethod,
			},
			Source: pokemonResultSourceResponse{
				Type:     resolvedResult.SourceType,
				UploadID: resolvedResult.UploadID,
				JobID:    resolvedResult.JobID,
				TimeRangeMS: pokemonResultTimeRangeMSResponse{
					Start: resolvedResult.StartMS,
					End:   resolvedResult.EndMS,
				},
				FrameTimestampMS: resolvedResult.FrameTimestampMS,
			},
			Confidence: resolvedResult.ExtractionConfidence,
			CreatedAt:  resolvedResult.CreatedAt.Format(time.RFC3339Nano),
		},
	})
}

type pokemonPendingSpeciesEnvelopeResponse struct {
	Readings []pokemonPendingReadingResponse `json:"readings"`
}

type pokemonPendingReadingResponse struct {
	ID         string                              `json:"id"`
	JobID      string                              `json:"jobId"`
	UploadID   string                              `json:"uploadId"`
	CP         int                                 `json:"cp"`
	HP         int                                 `json:"hp"`
	IVs        pokemonResultIVsResponse            `json:"ivs"`
	Level      pokemonResultLevelResponse          `json:"level"`
	Source     pokemonPendingReadingSourceResponse `json:"source"`
	Confidence *float64                            `json:"confidence"`
	Status     string                              `json:"status"`
	CreatedAt  string                              `json:"createdAt"`
	Options    []pokemonPendingOptionResponse      `json:"options"`
}

type pokemonPendingReadingSourceResponse struct {
	Type             string `json:"type"`
	FrameTimestampMS *int64 `json:"frameTimestampMs"`
}

type pokemonPendingOptionResponse struct {
	ID            string `json:"id"`
	SpeciesName   string `json:"speciesName"`
	MatchMode     string `json:"matchMode"`
	MatchDistance int    `json:"matchDistance"`
	OptionRank    int    `json:"optionRank"`
}

type pokemonPendingSpeciesResolveResponse struct {
	Result pokemonResultResponse `json:"result"`
}
