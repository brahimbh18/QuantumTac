package game

import (
	"database/sql"
	"os"
	"testing"

	"github.com/brahim/quantumtac-backend/internal/database"
)

func TestForfeitPersistsUserStats(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set, skipping integration test")
	}

	db, err := database.Connect(dsn)
	if err != nil {
		t.Fatalf("failed to connect to database: %v", err)
	}
	defer db.Close()

	if err := database.RunMigrations(db); err != nil {
		t.Fatalf("failed to run migrations: %v", err)
	}

	_, _ = db.Exec("DELETE FROM games_history")
	_, _ = db.Exec("DELETE FROM games")
	_, _ = db.Exec("DELETE FROM users")

	playerXID := seedUser(t, db, "player_x")
	playerOID := seedUser(t, db, "player_o")

	store := NewMemoryGameStore()
	repo := NewSQLHistoryRepository(db)
	service := NewServiceWithHistory(store, repo)

	game, err := service.CreateGame(playerXID)
	if err != nil {
		t.Fatalf("create game failed: %v", err)
	}

	game, err = service.JoinGame(game.ID, playerOID)
	if err != nil {
		t.Fatalf("join game failed: %v", err)
	}

	_, err = service.ForfeitGame(game.ID, playerXID)
	if err != nil {
		t.Fatalf("forfeit failed: %v", err)
	}

	var winsX, lossesX, winsO, lossesO int
	if err := db.QueryRow("SELECT wins, losses FROM users WHERE id = $1", playerXID).Scan(&winsX, &lossesX); err != nil {
		t.Fatalf("failed to query player X stats: %v", err)
	}
	if err := db.QueryRow("SELECT wins, losses FROM users WHERE id = $1", playerOID).Scan(&winsO, &lossesO); err != nil {
		t.Fatalf("failed to query player O stats: %v", err)
	}

	if winsX != 0 || lossesX != 1 {
		t.Fatalf("expected player X losses=1, wins=0, got wins=%d losses=%d", winsX, lossesX)
	}
	if winsO != 1 || lossesO != 0 {
		t.Fatalf("expected player O wins=1, losses=0, got wins=%d losses=%d", winsO, lossesO)
	}
}

func seedUser(t *testing.T, db *sql.DB, username string) int {
	var userID int
	err := db.QueryRow(
		`INSERT INTO users (username, password_hash) VALUES ($1, $2) RETURNING id`,
		username,
		"hash",
	).Scan(&userID)
	if err != nil {
		t.Fatalf("failed to seed user: %v", err)
	}
	return userID
}
