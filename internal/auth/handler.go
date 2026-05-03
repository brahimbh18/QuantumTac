package auth

import (
	"encoding/json"
	"errors"
	"net/http"
)

// Handler holds auth HTTP handlers.
type Handler struct {
	Service *Service
}

// NewHandler creates a new auth handler.
func NewHandler(service *Service) *Handler {
	return &Handler{Service: service}
}

type registerRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type tokenResponse struct {
	Token  string `json:"token"`
	UserID int    `json:"user_id"`
}

type userResponse struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
}

type errorResponse struct {
	Error string `json:"error"`
}

// Register handles POST /api/register.
func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return
	}

	if req.Username == "" || req.Password == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "username and password are required"})
		return
	}

	if len(req.Password) < 6 {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "password must be at least 6 characters"})
		return
	}

	user, err := h.Service.Register(req.Username, req.Password)
	if err != nil {
		if errors.Is(err, ErrUserExists) {
			writeJSON(w, http.StatusConflict, errorResponse{Error: "username already taken"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "internal server error"})
		return
	}

	writeJSON(w, http.StatusCreated, userResponse{ID: user.ID, Username: user.Username})
}

// Login handles POST /api/login.
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return
	}

	if req.Username == "" || req.Password == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "username and password are required"})
		return
	}

	user, token, err := h.Service.Login(req.Username, req.Password)
	if err != nil {
		if errors.Is(err, ErrInvalidCreds) {
			writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "invalid credentials"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "internal server error"})
		return
	}

	writeJSON(w, http.StatusOK, tokenResponse{Token: token, UserID: user.ID})
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}
