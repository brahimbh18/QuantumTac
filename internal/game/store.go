package game

import (
	"sync"
	"time"

	"github.com/brahim/quantumtac-backend/internal/models"
)

// GameStore defines operations for active games.
type GameStore interface {
	CreateGame(playerXID int) (models.Game, error)
	GetGame(gameID int) (models.Game, error)
	UpdateGame(gameID int, updater func(*models.Game) error) (models.Game, error)
	DeleteGame(gameID int) error
	ListGames() []models.Game
}

// MemoryGameStore stores games in memory.
type MemoryGameStore struct {
	mu     sync.RWMutex
	games  map[int]*models.Game
	nextID int
}

// NewMemoryGameStore creates a new in-memory game store.
func NewMemoryGameStore() *MemoryGameStore {
	return &MemoryGameStore{
		games: make(map[int]*models.Game),
	}
}

// Reset clears the store (for tests).
func (s *MemoryGameStore) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.games = make(map[int]*models.Game)
	s.nextID = 0
}

// CreateGame inserts a new game.
func (s *MemoryGameStore) CreateGame(playerXID int) (models.Game, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.nextID++
	now := time.Now()
	game := models.Game{
		ID:          s.nextID,
		PlayerXID:   playerXID,
		Board:       "---------",
		CurrentTurn: "X",
		Status:      models.StatusWaiting,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	stored := game
	s.games[game.ID] = &stored
	return game, nil
}

// GetGame returns a copy of a game by ID.
func (s *MemoryGameStore) GetGame(gameID int) (models.Game, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	game, ok := s.games[gameID]
	if !ok {
		return models.Game{}, ErrGameNotFound
	}

	return *game, nil
}

// UpdateGame mutates a game under lock.
func (s *MemoryGameStore) UpdateGame(gameID int, updater func(*models.Game) error) (models.Game, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	game, ok := s.games[gameID]
	if !ok {
		return models.Game{}, ErrGameNotFound
	}

	if err := updater(game); err != nil {
		return models.Game{}, err
	}

	return *game, nil
}

// DeleteGame removes a game.
func (s *MemoryGameStore) DeleteGame(gameID int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.games, gameID)
	return nil
}

// ListGames returns copies of all games.
func (s *MemoryGameStore) ListGames() []models.Game {
	s.mu.RLock()
	defer s.mu.RUnlock()

	games := make([]models.Game, 0, len(s.games))
	for _, game := range s.games {
		games = append(games, *game)
	}
	return games
}
