package main

import (
	"database/sql"
	"log"
	"net/http"
	"os"

	"github.com/brahim/quantumtac-backend/internal/auth"
	"github.com/brahim/quantumtac-backend/internal/database"
	"github.com/brahim/quantumtac-backend/internal/game"
	"github.com/brahim/quantumtac-backend/internal/health"
	"github.com/brahim/quantumtac-backend/internal/user"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func main() {
	if os.Getenv("JWT_SECRET") == "" {
		log.Fatal("JWT_SECRET environment variable is required")
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8081"
	}

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL environment variable is required")
	}

	db, err := database.Connect(dsn)
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer db.Close()

	if err := database.RunMigrations(db); err != nil {
		log.Fatalf("failed to run migrations: %v", err)
	}

	router := SetupRouter(db)

	log.Printf("server starting on :%s", port)
	if err := http.ListenAndServe(":"+port, router); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}

type RouterBundle struct {
	Router  *chi.Mux
	GameSvc *game.Service
}

// SetupRouter creates the chi router with all routes wired up.
// Exported so tests can reuse it.
func SetupRouter(db *sql.DB) *chi.Mux {
	b := SetupRouterWithServices(db)
	return b.Router
}

// SetupRouterWithServices returns the router plus the game service (for test resets).
func SetupRouterWithServices(db *sql.DB) RouterBundle {
	r := chi.NewRouter()
	r.Use(middleware.CleanPath)
	r.Use(middleware.StripSlashes)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)

	// Health
	healthHandler := health.NewHandler(db)
	r.Get("/health", healthHandler.Check)

	// Auth (public)
	authService := auth.NewService(db)
	authHandler := auth.NewHandler(authService)
	r.Post("/api/register", authHandler.Register)
	r.Post("/api/login", authHandler.Login)

	// Users (protected)
	userRepo := user.NewRepository(db)
	userHandler := user.NewHandler(userRepo)

	// Games (protected)
	gameStore := game.NewMemoryGameStore()
	gameHistory := game.NewSQLHistoryRepository(db)
	gameService := game.NewServiceFull(gameStore, gameHistory, userRepo)
	gameHandler := game.NewHandler(gameService)
	r.Group(func(r chi.Router) {
		r.Use(auth.Middleware)
		r.Get("/api/users/me/profile", userHandler.GetProfile)
		r.Post("/api/games", gameHandler.Create)
		r.Post("/api/games/queue", gameHandler.JoinQueue)
		r.Get("/api/games/active", gameHandler.GetActiveGame)
		r.Get("/api/games/{id}", gameHandler.Get)
		r.Post("/api/games/{id}/join", gameHandler.Join)
		r.Post("/api/games/{id}/move", gameHandler.Move)
		r.Post("/api/games/{id}/leave", gameHandler.Leave)
		r.Post("/api/games/{id}/forfeit", gameHandler.Forfeit)
	})

	return RouterBundle{Router: r, GameSvc: gameService}
}
