package httpserver

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/victoroliveirab/vibe-pokemongo-appraisal-app/web/internal/upload"
)

type pokemonDeleteHandler struct {
	store upload.Store
	now   func() time.Time
}

func newPokemonDeleteHandler(store upload.Store, nowFn func() time.Time) http.Handler {
	return &pokemonDeleteHandler{
		store: store,
		now:   nowFn,
	}
}

func (h *pokemonDeleteHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		w.Header().Set("Allow", http.MethodDelete)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	identity, ok := IdentityFromContext(r.Context())
	if !ok {
		writeAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error", nil)
		return
	}

	resultID := strings.TrimSpace(r.PathValue("resultId"))
	if resultID == "" {
		writeAPIError(w, http.StatusNotFound, "RESULT_NOT_FOUND", "Pokemon result not found", nil)
		return
	}

	err := h.store.SoftDeletePokemonResult(r.Context(), resultID, identity.OwnerKey(), h.now())
	if err != nil {
		if errors.Is(err, upload.ErrPokemonResultNotFound) {
			writeAPIError(w, http.StatusNotFound, "RESULT_NOT_FOUND", "Pokemon result not found", nil)
			return
		}

		writeAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error", nil)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
