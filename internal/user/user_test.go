package user

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/brahim/quantumtac-backend/internal/auth"
	"github.com/brahim/quantumtac-backend/internal/database"
	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
)

func TestProfileEndpoint(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://quantumtac:quantumtac@localhost:5432/quantumtac?sslmode=disable"
	}
	if os.Getenv("JWT_SECRET") == "" {
		os.Setenv("JWT_SECRET", "test-secret-key-for-testing-only")
	}

	db, err := database.Connect(dsn)
	if err != nil {
		t.Fatalf("failed to connect to database: %v", err)
	}
	defer db.Close()

	if err := database.RunMigrations(db); err != nil {
		t.Fatalf("failed to run migrations: %v", err)
	}

	if _, err := db.Exec("DELETE FROM games_history"); err != nil {
		_ = err
	}
	_, _ = db.Exec("DELETE FROM games")
	_, _ = db.Exec("DELETE FROM users")

	var userID int
	err = db.QueryRow(
		`INSERT INTO users (username, password_hash, wins, losses, draws)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id`,
		"stats_user",
		"hash",
		5,
		2,
		1,
	).Scan(&userID)
	if err != nil {
		t.Fatalf("failed to seed user: %v", err)
	}

	token, err := buildTestJWT(userID)
	if err != nil {
		t.Fatalf("failed to build jwt: %v", err)
	}

	repo := NewRepository(db)
	handler := NewHandler(repo)
	router := chi.NewRouter()
	router.Group(func(r chi.Router) {
		r.Use(auth.Middleware)
		r.Get("/api/users/me/profile", handler.GetProfile)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/users/me/profile", bytes.NewReader(nil))
	req.Header.Set("Authorization", "Bearer "+token)
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", res.Code, res.Body.String())
	}

	var profile Profile
	if err := json.NewDecoder(res.Body).Decode(&profile); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if profile.Username != "stats_user" || profile.Wins != 5 || profile.Losses != 2 || profile.Draws != 1 {
		t.Fatalf("unexpected profile: %+v", profile)
	}
}

func buildTestJWT(userID int) (string, error) {
	secret, err := auth.GetJWTSecret()
	if err != nil {
		return "", err
	}

	claims := jwt.MapClaims{
		"user_id": userID,
		"exp":     time.Now().Add(1 * time.Hour).Unix(),
		"iat":     time.Now().Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(secret)
}
