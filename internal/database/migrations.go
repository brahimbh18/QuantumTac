package database

import (
	"database/sql"
	"fmt"
	"log"
)

// RunMigrations creates the required tables if they don't exist.
func RunMigrations(db *sql.DB) error {
	migrations := []struct {
		name string
		stmt string
	}{
		{
			name: "create_users_table",
			stmt: `
				CREATE TABLE IF NOT EXISTS users (
					id            SERIAL PRIMARY KEY,
					username      VARCHAR(255) NOT NULL UNIQUE,
					password_hash TEXT         NOT NULL,
					wins          INTEGER      NOT NULL DEFAULT 0,
					losses        INTEGER      NOT NULL DEFAULT 0,
					draws         INTEGER      NOT NULL DEFAULT 0,
					created_at    TIMESTAMP    NOT NULL DEFAULT NOW()
				);`,
		},
		{
			name: "alter_users_add_stats",
			stmt: `
				ALTER TABLE users ADD COLUMN IF NOT EXISTS wins INTEGER NOT NULL DEFAULT 0;
				ALTER TABLE users ADD COLUMN IF NOT EXISTS losses INTEGER NOT NULL DEFAULT 0;
				ALTER TABLE users ADD COLUMN IF NOT EXISTS draws INTEGER NOT NULL DEFAULT 0;`,
		},
		{
			name: "create_games_table_v2",
			stmt: `
				CREATE TABLE IF NOT EXISTS games (
					id           SERIAL PRIMARY KEY,
					player_x_id  INTEGER      NOT NULL REFERENCES users(id),
					player_o_id  INTEGER               REFERENCES users(id),
					winner_id    INTEGER               REFERENCES users(id),
					board        VARCHAR(9)   NOT NULL DEFAULT '---------',
					current_turn VARCHAR(1)   NOT NULL DEFAULT 'X',
					status       VARCHAR(20)  NOT NULL DEFAULT 'waiting',
					created_at   TIMESTAMP    NOT NULL DEFAULT NOW(),
					updated_at   TIMESTAMP    NOT NULL DEFAULT NOW()
				);`,
		},
		{
			name: "create_games_history_table",
			stmt: `
				CREATE TABLE IF NOT EXISTS games_history (
					id           SERIAL PRIMARY KEY,
					game_id      INTEGER      NOT NULL UNIQUE,
					player_x_id  INTEGER      NOT NULL REFERENCES users(id),
					player_o_id  INTEGER               REFERENCES users(id),
					winner_id    INTEGER               REFERENCES users(id),
					board        VARCHAR(9)   NOT NULL,
					status       VARCHAR(20)  NOT NULL,
					created_at   TIMESTAMP    NOT NULL,
					finished_at  TIMESTAMP    NOT NULL DEFAULT NOW()
				);`,
		},
	}

	for _, m := range migrations {
		if _, err := db.Exec(m.stmt); err != nil {
			return fmt.Errorf("migration %q: %w", m.name, err)
		}
		log.Printf("migration %q applied", m.name)
	}

	return nil
}
