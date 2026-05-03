package auth

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/brahim/quantumtac-backend/internal/models"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrUserExists      = errors.New("username already taken")
	ErrInvalidCreds    = errors.New("invalid credentials")
	ErrMissingJWTKey   = errors.New("JWT_SECRET environment variable not set")
)

// Service handles authentication business logic.
type Service struct {
	DB *sql.DB
}

// NewService creates a new auth service.
func NewService(db *sql.DB) *Service {
	return &Service{DB: db}
}

// Register creates a new user with a bcrypt-hashed password.
func (s *Service) Register(username, password string) (models.User, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return models.User{}, fmt.Errorf("hash password: %w", err)
	}

	var user models.User
	err = s.DB.QueryRow(
		`INSERT INTO users (username, password_hash) VALUES ($1, $2)
		 RETURNING id, username, password_hash, created_at`,
		username, string(hash),
	).Scan(&user.ID, &user.Username, &user.PasswordHash, &user.CreatedAt)

	if err != nil {
		// Check for unique violation (PostgreSQL error code 23505).
		if isDuplicateKeyError(err) {
			return models.User{}, ErrUserExists
		}
		return models.User{}, fmt.Errorf("insert user: %w", err)
	}

	return user, nil
}

// Login verifies credentials and returns the user and a signed JWT.
func (s *Service) Login(username, password string) (models.User, string, error) {
	var user models.User
	err := s.DB.QueryRow(
		`SELECT id, username, password_hash, created_at FROM users WHERE username = $1`,
		username,
	).Scan(&user.ID, &user.Username, &user.PasswordHash, &user.CreatedAt)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return models.User{}, "", ErrInvalidCreds
		}
		return models.User{}, "", fmt.Errorf("query user: %w", err)
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return models.User{}, "", ErrInvalidCreds
	}

	token, err := generateJWT(user.ID)
	if err != nil {
		return models.User{}, "", err
	}

	return user, token, nil
}

// GetJWTSecret reads the secret from the environment.
func GetJWTSecret() ([]byte, error) {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		return nil, ErrMissingJWTKey
	}
	return []byte(secret), nil
}

func generateJWT(userID int) (string, error) {
	secret, err := GetJWTSecret()
	if err != nil {
		return "", err
	}

	claims := jwt.MapClaims{
		"user_id": userID,
		"exp":     time.Now().Add(24 * time.Hour).Unix(),
		"iat":     time.Now().Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(secret)
}

// isDuplicateKeyError checks if a PostgreSQL error is a unique violation.
func isDuplicateKeyError(err error) bool {
	return err != nil && (contains(err.Error(), "duplicate key") || contains(err.Error(), "23505"))
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
