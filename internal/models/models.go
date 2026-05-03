package models

import "time"

// User represents a registered player.
type User struct {
	ID           int       `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
}

// Game represents a TicTacToe match.
type Game struct {
	ID          int       `json:"id"`
	PlayerXID   int       `json:"player_x_id"`
	PlayerOID   *int      `json:"player_o_id,omitempty"`
	WinnerID    *int      `json:"winner_id,omitempty"`
	PlayerXName string    `json:"player_x_name"`
	PlayerOName string    `json:"player_o_name,omitempty"`
	Board       string    `json:"board"`
	CurrentTurn string    `json:"current_turn"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Game statuses.
const (
	StatusWaiting    = "waiting"
	StatusInProgress = "in_progress"
	StatusXWon       = "x_won"
	StatusOWon       = "o_won"
	StatusDraw       = "draw"
)
