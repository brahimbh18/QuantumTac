package game

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/brahim/quantumtac-backend/internal/auth"
	"github.com/go-chi/chi/v5"
)

type Handler struct {
	Service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{Service: service}
}

type moveRequest struct {
	Position int `json:"position"`
}

type errorResponse struct {
	Error string `json:"error"`
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "unauthorized"})
		return
	}
	game, err := h.Service.CreateGame(userID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "internal server error"})
		return
	}
	writeJSON(w, http.StatusCreated, game)
}

func (h *Handler) JoinQueue(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "unauthorized"})
		return
	}

	game, matched, err := h.Service.JoinQueue(userID)
	if err != nil {
		if errors.Is(err, ErrAlreadyInQueue) {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "internal server error"})
		return
	}

	if !matched {
		writeJSON(w, http.StatusAccepted, map[string]string{"status": "waiting"})
		return
	}

	writeJSON(w, http.StatusOK, game)
}

func (h *Handler) GetActiveGame(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "unauthorized"})
		return
	}

	game, err := h.Service.GetUserActiveGame(userID)
	if err != nil {
		if errors.Is(err, ErrGameNotFound) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "no active game found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "internal server error"})
		return
	}

	writeJSON(w, http.StatusOK, game)
}

func (h *Handler) Leave(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "unauthorized"})
		return
	}
	gameID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid game id"})
		return
	}
	_, err = h.Service.ForfeitGame(gameID, userID)
	if err != nil {
		handleGameError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "game forfeited"})
}

func (h *Handler) Forfeit(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "unauthorized"})
		return
	}
	gameID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid game id"})
		return
	}
	game, err := h.Service.ForfeitGame(gameID, userID)
	if err != nil {
		handleGameError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, game)
}

func (h *Handler) Join(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "unauthorized"})
		return
	}
	gameID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid game id"})
		return
	}
	game, err := h.Service.JoinGame(gameID, userID)
	if err != nil {
		handleGameError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, game)
}

func (h *Handler) Move(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "unauthorized"})
		return
	}
	gameID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid game id"})
		return
	}
	var req moveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return
	}
	game, err := h.Service.MakeMove(gameID, userID, req.Position)
	if err != nil {
		handleGameError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, game)
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	gameID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid game id"})
		return
	}
	game, err := h.Service.GetGame(gameID)
	if err != nil {
		handleGameError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, game)
}

func handleGameError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrGameNotFound):
		writeJSON(w, http.StatusNotFound, errorResponse{Error: err.Error()})
	case errors.Is(err, ErrGameFull), errors.Is(err, ErrNotYourTurn),
		errors.Is(err, ErrInvalidMove), errors.Is(err, ErrPositionTaken),
		errors.Is(err, ErrGameOver), errors.Is(err, ErrNotInGame),
		errors.Is(err, ErrCannotJoinOwn), errors.Is(err, ErrAlreadyInQueue):
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
	default:
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "internal server error"})
	}
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}
