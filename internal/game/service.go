package game

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/brahim/quantumtac-backend/internal/models"
)

var (
	ErrGameNotFound   = errors.New("game not found")
	ErrGameFull       = errors.New("game is already full")
	ErrNotYourTurn    = errors.New("not your turn")
	ErrInvalidMove    = errors.New("invalid move position")
	ErrPositionTaken  = errors.New("position already taken")
	ErrGameOver       = errors.New("game is already over")
	ErrNotInGame      = errors.New("you are not a player in this game")
	ErrCannotJoinOwn  = errors.New("cannot join your own game")
	ErrAlreadyInQueue = errors.New("already in queue")
)

// UsernameFetcher resolves a user ID to a username.
type UsernameFetcher interface {
	GetUsername(userID int) (string, error)
}

// Service handles TicTacToe game logic.
type Service struct {
	store         GameStore
	historyRepo   HistoryRepository
	usernames     UsernameFetcher
	mu            sync.Mutex
	queue         []int
	cleanupDelay  time.Duration
	cleanupTimers map[int]*time.Timer
}

// NewService creates a new game service.
func NewService(store GameStore) *Service {
	return &Service{
		store:         store,
		queue:         make([]int, 0),
		cleanupDelay:  30 * time.Second,
		cleanupTimers: make(map[int]*time.Timer),
	}
}

// NewServiceWithUsernames creates a game service that embeds player names.
func NewServiceWithUsernames(store GameStore, fetcher UsernameFetcher) *Service {
	return &Service{
		store:         store,
		usernames:     fetcher,
		queue:         make([]int, 0),
		cleanupDelay:  30 * time.Second,
		cleanupTimers: make(map[int]*time.Timer),
	}
}

// NewServiceWithHistory creates a new game service with history persistence.
func NewServiceWithHistory(store GameStore, repo HistoryRepository) *Service {
	service := NewService(store)
	service.historyRepo = repo
	return service
}

// NewServiceFull creates a game service with all dependencies.
func NewServiceFull(store GameStore, repo HistoryRepository, fetcher UsernameFetcher) *Service {
	return &Service{
		store:         store,
		historyRepo:   repo,
		usernames:     fetcher,
		queue:         make([]int, 0),
		cleanupDelay:  30 * time.Second,
		cleanupTimers: make(map[int]*time.Timer),
	}
}

// ResetQueue clears the in-memory matchmaking queue (for testing).
func (s *Service) ResetQueue() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.queue = make([]int, 0)
}

// ResetStore clears the store (for tests).
func (s *Service) ResetStore() {
	if resetter, ok := s.store.(interface{ Reset() }); ok {
		resetter.Reset()
	}

	s.mu.Lock()
	for _, timer := range s.cleanupTimers {
		timer.Stop()
	}
	s.cleanupTimers = make(map[int]*time.Timer)
	s.mu.Unlock()
}

// JoinQueue adds a player to the queue and returns a game if matched.
// Destructive Entry: any existing active game is forfeited before queueing.
func (s *Service) JoinQueue(userID int) (models.Game, bool, error) {
	// Step 1: Resolve stale/active game (non-blocking).
	if activeGame, err := s.GetUserActiveGame(userID); err == nil {
		if _, leaveErr := s.ForfeitGame(activeGame.ID, userID); leaveErr != nil {
			// Log but do not block — forfeit is idempotent on terminal games.
			fmt.Printf("joinQueue: leave old game %d for user %d failed: %v\n", activeGame.ID, userID, leaveErr)
		} else {
			fmt.Printf("joinQueue: forfeited game %d for user %d\n", activeGame.ID, userID)
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Step 2: Add to matchmaking queue.
	for _, id := range s.queue {
		if id == userID {
			return models.Game{}, false, ErrAlreadyInQueue
		}
	}

	if len(s.queue) == 0 {
		s.queue = append(s.queue, userID)
		return models.Game{}, false, nil
	}

	playerXID := s.queue[0]
	s.queue = s.queue[1:]

	game, err := s.CreateGame(playerXID)
	if err != nil {
		return models.Game{}, false, err
	}

	game, err = s.JoinGame(game.ID, userID)
	if err != nil {
		return models.Game{}, false, err
	}

	return game, true, nil
}

// GetUserActiveGame checks if a user is currently in an active game.
// If a game is found but has been inactive for more than 5 minutes, it's forfeited.
func (s *Service) GetUserActiveGame(userID int) (models.Game, error) {
	games := s.store.ListGames()
	var latest *models.Game
	for i := range games {
		game := games[i]
		if game.PlayerXID != userID && (game.PlayerOID == nil || *game.PlayerOID != userID) {
			continue
		}
		if game.Status != models.StatusInProgress && game.Status != models.StatusWaiting {
			continue
		}
		if latest == nil || game.UpdatedAt.After(latest.UpdatedAt) {
			copy := game
			latest = &copy
		}
	}

	if latest == nil {
		return models.Game{}, ErrGameNotFound
	}

	if time.Since(latest.UpdatedAt) > 5*time.Minute {
		_, _ = s.forfeitWithWinner(latest.ID, userID)
		return models.Game{}, ErrGameNotFound
	}

	s.hydrateNames(latest)
	return *latest, nil
}

// ForfeitGame marks a game as terminal and assigns the OTHER player as the winner.
func (s *Service) ForfeitGame(gameID, userID int) (models.Game, error) {
	game, err := s.forfeitWithWinner(gameID, userID)
	if err != nil {
		return models.Game{}, err
	}
	return game, nil
}

// CreateGame starts a new game with the given player as X.
func (s *Service) CreateGame(playerXID int) (models.Game, error) {
	game, err := s.store.CreateGame(playerXID)
	if err != nil {
		return game, err
	}
	if s.usernames != nil {
		if name, err := s.usernames.GetUsername(playerXID); err == nil {
			game, _ = s.store.UpdateGame(game.ID, func(g *models.Game) error {
				g.PlayerXName = name
				return nil
			})
		}
	}
	return game, nil
}

// JoinGame adds a second player (O) to a waiting game.
func (s *Service) JoinGame(gameID, playerOID int) (models.Game, error) {
	return s.store.UpdateGame(gameID, func(game *models.Game) error {
		if game.Status != models.StatusWaiting {
			return ErrGameFull
		}
		if game.PlayerXID == playerOID {
			return ErrCannotJoinOwn
		}
		game.PlayerOID = &playerOID
		game.Status = models.StatusInProgress
		game.UpdatedAt = time.Now()
		if s.usernames != nil {
			if name, err := s.usernames.GetUsername(playerOID); err == nil {
				game.PlayerOName = name
			}
		}
		return nil
	})
}

// MakeMove places a mark on the board at the given position (0-8).
func (s *Service) MakeMove(gameID, playerID, position int) (models.Game, error) {
	if position < 0 || position > 8 {
		return models.Game{}, ErrInvalidMove
	}

	updated, err := s.store.UpdateGame(gameID, func(game *models.Game) error {
		if game.Status != models.StatusInProgress {
			return ErrGameOver
		}

		var mark string
		switch playerID {
		case game.PlayerXID:
			mark = "X"
		default:
			if game.PlayerOID != nil && *game.PlayerOID == playerID {
				mark = "O"
			} else {
				return ErrNotInGame
			}
		}

		if game.CurrentTurn != mark {
			return ErrNotYourTurn
		}

		if game.Board[position] != '-' {
			return ErrPositionTaken
		}

		board := []byte(game.Board)
		board[position] = mark[0]
		newBoard := string(board)

		newStatus := game.Status
		nextTurn := game.CurrentTurn
		var winnerID *int

		winner := checkWinner(newBoard)
		switch winner {
		case "X":
			newStatus = models.StatusXWon
			winnerID = &game.PlayerXID
		case "O":
			newStatus = models.StatusOWon
			winnerID = game.PlayerOID
		case "draw":
			newStatus = models.StatusDraw
		default:
			if game.CurrentTurn == "X" {
				nextTurn = "O"
			} else {
				nextTurn = "X"
			}
		}

		game.Board = newBoard
		game.CurrentTurn = nextTurn
		game.Status = newStatus
		game.WinnerID = winnerID
		game.UpdatedAt = time.Now()
		return nil
	})
	if err != nil {
		return models.Game{}, err
	}

	if updated.Status == models.StatusXWon || updated.Status == models.StatusOWon || updated.Status == models.StatusDraw {
		s.persistHistory(updated)
		s.scheduleCleanup(updated.ID)
	}

	return updated, nil
}

// GetGame fetches a game by ID, hydrating player names if missing.
func (s *Service) GetGame(gameID int) (models.Game, error) {
	game, err := s.store.GetGame(gameID)
	if err != nil {
		return game, err
	}
	s.hydrateNames(&game)
	return game, nil
}

func (s *Service) hydrateNames(game *models.Game) {
	if s.usernames == nil {
		return
	}
	if game.PlayerXName == "" {
		if name, err := s.usernames.GetUsername(game.PlayerXID); err == nil {
			game.PlayerXName = name
		}
	}
	if game.PlayerOName == "" && game.PlayerOID != nil {
		if name, err := s.usernames.GetUsername(*game.PlayerOID); err == nil {
			game.PlayerOName = name
		}
	}
}

func (s *Service) forfeitWithWinner(gameID, userID int) (models.Game, error) {
	updated, err := s.store.UpdateGame(gameID, func(game *models.Game) error {
		if game.Status != models.StatusInProgress && game.Status != models.StatusWaiting {
			return ErrGameOver
		}
		if game.PlayerXID != userID && (game.PlayerOID == nil || *game.PlayerOID != userID) {
			return ErrNotInGame
		}

		var winnerID *int
		if game.PlayerXID == userID {
			winnerID = game.PlayerOID
			if winnerID != nil {
				game.Status = models.StatusOWon
			} else {
				game.Status = models.StatusDraw
			}
		} else {
			winnerID = &game.PlayerXID
			game.Status = models.StatusXWon
		}

		game.WinnerID = winnerID
		game.UpdatedAt = time.Now()
		return nil
	})
	if err != nil {
		return models.Game{}, err
	}

	if updated.Status == models.StatusXWon || updated.Status == models.StatusOWon || updated.Status == models.StatusDraw {
		s.persistHistory(updated)
		s.scheduleCleanup(updated.ID)
	}

	return updated, nil
}

func (s *Service) scheduleCleanup(gameID int) {
	s.mu.Lock()
	if timer, ok := s.cleanupTimers[gameID]; ok {
		timer.Stop()
	}
	s.cleanupTimers[gameID] = time.AfterFunc(s.cleanupDelay, func() {
		_ = s.store.DeleteGame(gameID)
		s.mu.Lock()
		delete(s.cleanupTimers, gameID)
		s.mu.Unlock()
	})
	s.mu.Unlock()
}

func (s *Service) persistHistory(game models.Game) {
	if s.historyRepo == nil {
		return
	}
	_ = s.historyRepo.SaveGameHistory(context.Background(), game)
}

// checkWinner evaluates the board and returns "X", "O", "draw", or "" (ongoing).
func checkWinner(board string) string {
	// All winning line indices.
	lines := [][3]int{
		{0, 1, 2}, {3, 4, 5}, {6, 7, 8}, // rows
		{0, 3, 6}, {1, 4, 7}, {2, 5, 8}, // columns
		{0, 4, 8}, {2, 4, 6}, // diagonals
	}

	for _, line := range lines {
		a, b, c := board[line[0]], board[line[1]], board[line[2]]
		if a != '-' && a == b && b == c {
			return string(a)
		}
	}

	// Check for draw (no empty cells left).
	for _, cell := range board {
		if cell == '-' {
			return "" // game still ongoing
		}
	}

	return "draw"
}
