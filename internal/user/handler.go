package user

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/brahim/quantumtac-backend/internal/auth"
)

// Handler holds user HTTP handlers.
type Handler struct {
	Repo *Repository
}

// NewHandler creates a new user handler.
func NewHandler(repo *Repository) *Handler {
	return &Handler{Repo: repo}
}

type errorResponse struct {
	Error string `json:"error"`
}

// GetProfile handles GET /api/users/me/profile.
func (h *Handler) GetProfile(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "unauthorized"})
		return
	}

	if h.Repo == nil || h.Repo.DB == nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "profile service unavailable"})
		return
	}

	profile, err := h.Repo.GetProfile(userID)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "user not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "internal server error"})
		return
	}

	writeJSON(w, http.StatusOK, profile)
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}
