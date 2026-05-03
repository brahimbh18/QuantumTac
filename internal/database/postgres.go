package database

import (
	"database/sql"
	"fmt"

	_ "github.com/lib/pq"
)

// Connect opens a PostgreSQL connection pool and verifies connectivity.
func Connect(dsn string) (*sql.DB, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("database open: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("database ping: %w", err)
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)

	return db, nil
}
