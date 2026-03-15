package httpserver

import (
	"net/http"
	"time"

	"github.com/victoroliveirab/vibe-pokemongo-appraisal-app/web/internal/upload"
)

type pokemonResultsHandler struct {
	store upload.Store
}

func newPokemonResultsHandler(store upload.Store) http.Handler {
	return &pokemonResultsHandler{store: store}
}

func (h *pokemonResultsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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

	results, err := h.store.ListPokemonResultsBySession(r.Context(), sess.ID)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error", nil)
		return
	}

	payloadResults := make([]pokemonResultResponse, 0, len(results))
	for _, result := range results {
		payloadMaxCPEvaluations := make([]pokemonResultMaxCPEvaluationResponse, 0, len(result.MaxCPEvaluations))
		for _, evaluation := range result.MaxCPEvaluations {
			payloadMaxCPEvaluations = append(payloadMaxCPEvaluations, pokemonResultMaxCPEvaluationResponse{
				MaxCP:              evaluation.MaxCP,
				EvaluatedSpeciesID: evaluation.EvaluatedSpeciesID,
				BestLevel:          evaluation.BestLevel,
				BestCP:             evaluation.BestCP,
				StatProduct:        evaluation.StatProduct,
				Rank:               evaluation.Rank,
				Percentage:         evaluation.Percentage,
			})
		}

		payloadResults = append(payloadResults, pokemonResultResponse{
			ID:                  result.ID,
			SpeciesName:         result.SpeciesName,
			CP:                  result.CP,
			HP:                  result.HP,
			PowerUpStardustCost: result.PowerUpStardustCost,
			IVs: pokemonResultIVsResponse{
				Attack:  result.IVAttack,
				Defense: result.IVDefense,
				Stamina: result.IVStamina,
			},
			Level: pokemonResultLevelResponse{
				Estimate:   result.LevelEstimate,
				Confidence: result.LevelConfidence,
				Method:     result.LevelMethod,
			},
			Source: pokemonResultSourceResponse{
				Type:     result.SourceType,
				UploadID: result.UploadID,
				JobID:    result.JobID,
				TimeRangeMS: pokemonResultTimeRangeMSResponse{
					Start: result.StartMS,
					End:   result.EndMS,
				},
				FrameTimestampMS: result.FrameTimestampMS,
			},
			Confidence:       result.ExtractionConfidence,
			MaxCPEvaluations: payloadMaxCPEvaluations,
			CreatedAt:        result.CreatedAt.Format(time.RFC3339Nano),
		})
	}

	writeJSON(w, http.StatusOK, pokemonResultsEnvelopeResponse{
		Results: payloadResults,
	})
}

type pokemonResultsEnvelopeResponse struct {
	Results []pokemonResultResponse `json:"results"`
}

type pokemonResultResponse struct {
	ID                  string                                 `json:"id"`
	SpeciesName         string                                 `json:"speciesName"`
	CP                  int                                    `json:"cp"`
	HP                  int                                    `json:"hp"`
	PowerUpStardustCost int                                    `json:"powerUpStardustCost"`
	IVs                 pokemonResultIVsResponse               `json:"ivs"`
	Level               pokemonResultLevelResponse             `json:"level"`
	Source              pokemonResultSourceResponse            `json:"source"`
	Confidence          *float64                               `json:"confidence"`
	MaxCPEvaluations    []pokemonResultMaxCPEvaluationResponse `json:"maxCpEvaluations,omitempty"`
	CreatedAt           string                                 `json:"createdAt"`
}

type pokemonResultMaxCPEvaluationResponse struct {
	MaxCP              int     `json:"maxCp"`
	EvaluatedSpeciesID string  `json:"evaluatedSpeciesId"`
	BestLevel          float64 `json:"bestLevel"`
	BestCP             int     `json:"bestCp"`
	StatProduct        float64 `json:"statProduct"`
	Rank               int     `json:"rank"`
	Percentage         float64 `json:"percentage"`
}

type pokemonResultIVsResponse struct {
	Attack  int `json:"attack"`
	Defense int `json:"defense"`
	Stamina int `json:"stamina"`
}

type pokemonResultLevelResponse struct {
	Estimate   *float64 `json:"estimate"`
	Confidence *float64 `json:"confidence"`
	Method     string   `json:"method"`
}

type pokemonResultSourceResponse struct {
	Type             string                           `json:"type"`
	UploadID         string                           `json:"uploadId"`
	JobID            string                           `json:"jobId"`
	TimeRangeMS      pokemonResultTimeRangeMSResponse `json:"timeRangeMs"`
	FrameTimestampMS *int64                           `json:"frameTimestampMs"`
}

type pokemonResultTimeRangeMSResponse struct {
	Start *int64 `json:"start"`
	End   *int64 `json:"end"`
}
