package game

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/brahim/quantumtac-backend/internal/models"
)

// HistoryRepository persists completed games and user stats.
type HistoryRepository interface {
	SaveGameHistory(ctx context.Context, game models.Game) error
}

// SQLHistoryRepository stores history and stats in Postgres.
type SQLHistoryRepository struct {
	DB *sql.DB
}

// NewSQLHistoryRepository creates a SQL history repository.
func NewSQLHistoryRepository(db *sql.DB) *SQLHistoryRepository {
	return &SQLHistoryRepository{DB: db}
}

// SaveGameHistory records a terminal game and updates user stats atomically.
func (r *SQLHistoryRepository) SaveGameHistory(ctx context.Context, game models.Game) error {
	if r == nil || r.DB == nil {
		return nil
	}

	if game.Status != models.StatusXWon && game.Status != models.StatusOWon && game.Status != models.StatusDraw {
		return nil
	}

	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	var historyID int
	err = tx.QueryRowContext(
		ctx,
		`INSERT INTO games_history (game_id, player_x_id, player_o_id, winner_id, board, status, created_at, finished_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 ON CONFLICT (game_id) DO NOTHING
		 RETURNING id`,
		game.ID,
		game.PlayerXID,
		game.PlayerOID,
		game.WinnerID,
		game.Board,
		game.Status,
		game.CreatedAt,
		time.Now(),
	).Scan(&historyID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return tx.Commit()
		}
		return err
	}

	if err := updateStats(tx, game); err != nil {
		return err
	}

	return tx.Commit()
}

func updateStats(tx *sql.Tx, game models.Game) error {
	if game.PlayerOID == nil {
		return nil
	}

	switch game.Status {
	case models.StatusXWon:
		if _, err := tx.Exec(`UPDATE users SET wins = wins + 1 WHERE id = $1`, game.PlayerXID); err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE users SET losses = losses + 1 WHERE id = $1`, *game.PlayerOID); err != nil {
			return err
		}
	case models.StatusOWon:
		if _, err := tx.Exec(`UPDATE users SET wins = wins + 1 WHERE id = $1`, *game.PlayerOID); err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE users SET losses = losses + 1 WHERE id = $1`, game.PlayerXID); err != nil {
			return err
		}
	case models.StatusDraw:
		if _, err := tx.Exec(`UPDATE users SET draws = draws + 1 WHERE id IN ($1, $2)`, game.PlayerXID, *game.PlayerOID); err != nil {
			return err
		}
	}

	return nil
}
