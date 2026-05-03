package auth

import (
	"context"
	"log"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

type contextKey string

const UserIDKey contextKey = "userID"

// Middleware validates JWT tokens and injects the user ID into the request context.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "missing authorization header"})
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
			writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "invalid authorization format"})
			return
		}

		secret, err := GetJWTSecret()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "internal server error"})
			return
		}

		token, err := jwt.Parse(parts[1], func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, jwt.ErrSignatureInvalid
			}
			return secret, nil
		})

		if err != nil || !token.Valid {
			if err != nil {
				log.Printf("auth: jwt parse failed: %v", err)
			} else {
				log.Printf("auth: jwt invalid")
			}
			writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "invalid or expired token"})
			return
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "invalid token claims"})
			return
		}

		userIDFloat, ok := claims["user_id"].(float64)
		if !ok {
			writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "invalid token claims"})
			return
		}

		ctx := context.WithValue(r.Context(), UserIDKey, int(userIDFloat))
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// UserIDFromContext extracts the user ID from the request context.
func UserIDFromContext(ctx context.Context) (int, bool) {
	id, ok := ctx.Value(UserIDKey).(int)
	return id, ok
}
