package httpserver

import (
	"encoding/json"
	"net/http"
)

// APIError defines the shared API error envelope.
type APIError struct {
	Error struct {
		Code    string      `json:"code"`
		Message string      `json:"message"`
		Details interface{} `json:"details,omitempty"`
	} `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(payload); err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
	}
}

func writeAPIError(w http.ResponseWriter, status int, code, message string, details interface{}) {
	resp := APIError{}
	resp.Error.Code = code
	resp.Error.Message = message
	resp.Error.Details = details

	writeJSON(w, status, resp)
}
