package user

import (
	"database/sql"
	"errors"
	"fmt"
)

// Profile holds public profile stats.
type Profile struct {
	Username string `json:"username"`
	Wins     int    `json:"wins"`
	Losses   int    `json:"losses"`
	Draws    int    `json:"draws"`
}

// Repository provides access to user data.
type Repository struct {
	DB *sql.DB
}

// NewRepository creates a user repository.
func NewRepository(db *sql.DB) *Repository {
	return &Repository{DB: db}
}

// GetUsername returns the username for a given user ID.
func (r *Repository) GetUsername(userID int) (string, error) {
	if r.DB == nil {
		return "", ErrUserNotFound
	}
	var username string
	err := r.DB.QueryRow(`SELECT username FROM users WHERE id = $1`, userID).Scan(&username)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrUserNotFound
		}
		return "", fmt.Errorf("get username: %w", err)
	}
	return username, nil
}

// GetProfile fetches a user's public profile stats.
func (r *Repository) GetProfile(userID int) (Profile, error) {
	var profile Profile
	err := r.DB.QueryRow(
		`SELECT username, wins, losses, draws FROM users WHERE id = $1`,
		userID,
	).Scan(&profile.Username, &profile.Wins, &profile.Losses, &profile.Draws)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Profile{}, ErrUserNotFound
		}
		return Profile{}, fmt.Errorf("get profile: %w", err)
	}
	return profile, nil
}

var ErrUserNotFound = errors.New("user not found")
