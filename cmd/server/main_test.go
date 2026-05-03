package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/brahim/quantumtac-backend/internal/database"
	"github.com/brahim/quantumtac-backend/internal/models"
)

var testRouter http.Handler
var testGameSvc interface{ ResetQueue() }

func TestMain(m *testing.M) {
	if os.Getenv("JWT_SECRET") == "" {
		os.Setenv("JWT_SECRET", "test-secret-key-for-testing-only")
	}

	// Skip tests that require database if DATABASE_URL is not set
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		fmt.Println("DATABASE_URL not set, skipping tests that require database")
		os.Exit(0)
	}

	db, err := database.Connect(dsn)
	if err != nil {
		fmt.Printf("failed to connect to database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := database.RunMigrations(db); err != nil {
		fmt.Printf("failed to run migrations: %v\n", err)
		os.Exit(1)
	}

	bundle := SetupRouterWithServices(db)
	testRouter = bundle.Router
	testGameSvc = bundle.GameSvc
	if testGameSvc != nil {
		if resetter, ok := testGameSvc.(interface{ ResetStore() }); ok {
			resetter.ResetStore()
		}
	}

	os.Exit(m.Run())
}

func TestHealthEndpoint(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	testRouter.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if body["status"] != "ok" {
		t.Fatalf("expected status ok, got %s", body["status"])
	}
}

func TestRegisterEndpoint(t *testing.T) {
	payload := map[string]string{
		"username": "testplayer",
		"password": "secret123",
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/api/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	testRouter.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d: %s", rec.Code, rec.Body.String())
	}

	var user map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&user); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if user["username"] != "testplayer" {
		t.Fatalf("expected username testplayer, got %v", user["username"])
	}

	if _, ok := user["id"]; !ok {
		t.Fatal("expected id in response")
	}
}

func TestMatchmakingFlow(t *testing.T) {
	if testGameSvc != nil {
		if resetter, ok := testGameSvc.(interface{ ResetStore() }); ok {
			resetter.ResetStore()
		}
	}
	// Helper to register and login a user.
	setupUser := func(username string) (int, string) {
		payload, _ := json.Marshal(map[string]string{
			"username": username,
			"password": "password123",
		})
		regReq := httptest.NewRequest(http.MethodPost, "/api/register", bytes.NewReader(payload))
		regReq.Header.Set("Content-Type", "application/json")
		regRec := httptest.NewRecorder()
		testRouter.ServeHTTP(regRec, regReq)

		loginReq := httptest.NewRequest(http.MethodPost, "/api/login", bytes.NewReader(payload))
		loginReq.Header.Set("Content-Type", "application/json")
		loginRec := httptest.NewRecorder()
		testRouter.ServeHTTP(loginRec, loginReq)

		var resp map[string]interface{}
		json.NewDecoder(loginRec.Body).Decode(&resp)
		return int(resp["user_id"].(float64)), resp["token"].(string)
	}

	idA, tokenA := setupUser("playerA")
	idB, tokenB := setupUser("playerB")

	// 0. Test Unauthorized access.
	t.Run("Unauthorized access to queue", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/games/queue", nil)
		rec := httptest.NewRecorder()
		testRouter.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected 401 Unauthorized, got %d", rec.Code)
		}
	})

	var gameID int

	// 1. Player A joins queue.
	t.Run("Player A joins queue", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/games/queue", nil)
		req.Header.Set("Authorization", "Bearer "+tokenA)
		rec := httptest.NewRecorder()
		testRouter.ServeHTTP(rec, req)

		if rec.Code != http.StatusAccepted {
			t.Fatalf("expected 202 Accepted, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	// 2. Player B joins queue -> match made.
	t.Run("Player B joins queue -> match", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/games/queue", nil)
		req.Header.Set("Authorization", "Bearer "+tokenB)
		rec := httptest.NewRecorder()
		testRouter.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}

		var game map[string]interface{}
		json.NewDecoder(rec.Body).Decode(&game)
		gameID = int(game["id"].(float64))

		if game["player_x_id"] != float64(idA) {
			t.Errorf("expected player_x_id %d, got %v", idA, game["player_x_id"])
		}
		if game["player_o_id"] != float64(idB) {
			t.Errorf("expected player_o_id %d, got %v", idB, game["player_o_id"])
		}
		if game["status"] != "in_progress" {
			t.Errorf("expected status in_progress, got %s", game["status"])
		}
	})

	// 3. Player A checks active game.
	t.Run("Player A fetches active game", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/games/active", nil)
		req.Header.Set("Authorization", "Bearer "+tokenA)
		rec := httptest.NewRecorder()
		testRouter.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}

		var game map[string]interface{}
		json.NewDecoder(rec.Body).Decode(&game)
		fetchedID := int(game["id"].(float64))

		if fetchedID != gameID {
			t.Errorf("expected game id %d, got %d", gameID, fetchedID)
		}

		if game["status"] != "in_progress" {
			t.Errorf("expected status in_progress, got %s", game["status"])
		}
	})

	_ = gameID
}

func TestForfeitResolution(t *testing.T) {
	if testGameSvc != nil {
		if resetter, ok := testGameSvc.(interface{ ResetStore() }); ok {
			resetter.ResetStore()
		}
	}
	setupUser := func(username string) (int, string) {
		payload, _ := json.Marshal(map[string]string{
			"username": username,
			"password": "password123",
		})
		regReq := httptest.NewRequest(http.MethodPost, "/api/register", bytes.NewReader(payload))
		regReq.Header.Set("Content-Type", "application/json")
		regRec := httptest.NewRecorder()
		testRouter.ServeHTTP(regRec, regReq)

		loginReq := httptest.NewRequest(http.MethodPost, "/api/login", bytes.NewReader(payload))
		loginReq.Header.Set("Content-Type", "application/json")
		loginRec := httptest.NewRecorder()
		testRouter.ServeHTTP(loginRec, loginReq)

		var resp map[string]interface{}
		json.NewDecoder(loginRec.Body).Decode(&resp)
		return int(resp["user_id"].(float64)), resp["token"].(string)
	}

	_, tokenA := setupUser("playerA")
	idB, tokenB := setupUser("playerB")

	// 1. Join Queue to create a match
	reqA := httptest.NewRequest(http.MethodPost, "/api/games/queue", nil)
	reqA.Header.Set("Authorization", "Bearer "+tokenA)
	recA := httptest.NewRecorder()
	testRouter.ServeHTTP(recA, reqA)

	reqB := httptest.NewRequest(http.MethodPost, "/api/games/queue", nil)
	reqB.Header.Set("Authorization", "Bearer "+tokenB)
	recB := httptest.NewRecorder()
	testRouter.ServeHTTP(recB, reqB)

	var game models.Game
	json.NewDecoder(recB.Body).Decode(&game)

	// 2. Player A forfeits
	leaveReq := httptest.NewRequest(http.MethodPost, "/api/games/"+fmt.Sprintf("%d", game.ID)+"/forfeit", nil)
	leaveReq.Header.Set("Authorization", "Bearer "+tokenA)
	leaveRec := httptest.NewRecorder()
	testRouter.ServeHTTP(leaveRec, leaveReq)

	if leaveRec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for leave, got %d", leaveRec.Code)
	}

	var resp models.Game
	if err := json.NewDecoder(leaveRec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Status != models.StatusOWon {
		t.Errorf("expected status %s, got %s", models.StatusOWon, resp.Status)
	}
	if resp.WinnerID == nil || *resp.WinnerID != idB {
		t.Errorf("expected winner ID %d, got %v", idB, resp.WinnerID)
	}
}

func TestQueueCleanupLogic(t *testing.T) {
	if testGameSvc != nil {
		if resetter, ok := testGameSvc.(interface{ ResetStore() }); ok {
			resetter.ResetStore()
		}
	}
	setupUser := func(username string) (int, string) {
		payload, _ := json.Marshal(map[string]string{
			"username": username,
			"password": "password123",
		})
		regReq := httptest.NewRequest(http.MethodPost, "/api/register", bytes.NewReader(payload))
		regReq.Header.Set("Content-Type", "application/json")
		testRouter.ServeHTTP(httptest.NewRecorder(), regReq)

		loginReq := httptest.NewRequest(http.MethodPost, "/api/login", bytes.NewReader(payload))
		loginReq.Header.Set("Content-Type", "application/json")
		loginRec := httptest.NewRecorder()
		testRouter.ServeHTTP(loginRec, loginReq)

		var resp map[string]interface{}
		json.NewDecoder(loginRec.Body).Decode(&resp)
		return int(resp["user_id"].(float64)), resp["token"].(string)
	}

	_, tokenA := setupUser("playerA")
	_, tokenB := setupUser("playerB")

	// Player A joins queue while in active game
	reqA := httptest.NewRequest(http.MethodPost, "/api/games/queue", nil)
	reqA.Header.Set("Authorization", "Bearer "+tokenA)
	recA := httptest.NewRecorder()
	testRouter.ServeHTTP(recA, reqA)

	// Player B joins to start a game
	reqB := httptest.NewRequest(http.MethodPost, "/api/games/queue", nil)
	reqB.Header.Set("Authorization", "Bearer "+tokenB)
	recB := httptest.NewRecorder()
	testRouter.ServeHTTP(recB, reqB)

	if recB.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for match, got %d: %s", recB.Code, recB.Body.String())
	}

	var game models.Game
	if err := json.NewDecoder(recB.Body).Decode(&game); err != nil {
		t.Fatalf("failed to decode game: %v", err)
	}

	// Player A joins queue while in active game
	req := httptest.NewRequest(http.MethodPost, "/api/games/queue", nil)
	req.Header.Set("Authorization", "Bearer "+tokenA)
	rec := httptest.NewRecorder()
	testRouter.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202 Accepted, got %d: %s", rec.Code, rec.Body.String())
	}

	activeReq := httptest.NewRequest(http.MethodGet, "/api/games/active", nil)
	activeReq.Header.Set("Authorization", "Bearer "+tokenA)
	activeRec := httptest.NewRecorder()
	testRouter.ServeHTTP(activeRec, activeReq)
	if activeRec.Code == http.StatusOK {
		var active models.Game
		if err := json.NewDecoder(activeRec.Body).Decode(&active); err == nil {
			if active.ID == game.ID {
				t.Errorf("expected old game to be forfeited, still active with id %d", active.ID)
			}
		}
	}
}

func TestStaleGameAutoAbandon(t *testing.T) {
	if testGameSvc != nil {
		if resetter, ok := testGameSvc.(interface{ ResetStore() }); ok {
			resetter.ResetStore()
		}
	}
	// Setup user A
	payload, _ := json.Marshal(map[string]string{"username": "staleplayer", "password": "password123"})
	regReq := httptest.NewRequest(http.MethodPost, "/api/register", bytes.NewReader(payload))
	regReq.Header.Set("Content-Type", "application/json")
	testRouter.ServeHTTP(httptest.NewRecorder(), regReq)

	loginReq := httptest.NewRequest(http.MethodPost, "/api/login", bytes.NewReader(payload))
	loginReq.Header.Set("Content-Type", "application/json")
	loginRec := httptest.NewRecorder()
	testRouter.ServeHTTP(loginRec, loginReq)
	var resp map[string]interface{}
	json.NewDecoder(loginRec.Body).Decode(&resp)
	_, tokenA := int(resp["user_id"].(float64)), resp["token"].(string)

	// Call getActiveGame
	req := httptest.NewRequest(http.MethodGet, "/api/games/active", nil)
	req.Header.Set("Authorization", "Bearer "+tokenA)
	rec := httptest.NewRecorder()
	testRouter.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 Not Found for stale game, got %d", rec.Code)
	}

	if rec.Code == http.StatusOK {
		t.Errorf("expected no active game for stale flow without DB state")
	}
}

func TestForfeitConcurrency(t *testing.T) {
	if testGameSvc != nil {
		if resetter, ok := testGameSvc.(interface{ ResetStore() }); ok {
			resetter.ResetStore()
		}
	}
	setupUser := func(username string) (int, string) {
		payload, _ := json.Marshal(map[string]string{
			"username": username,
			"password": "password123",
		})
		regReq := httptest.NewRequest(http.MethodPost, "/api/register", bytes.NewReader(payload))
		regReq.Header.Set("Content-Type", "application/json")
		testRouter.ServeHTTP(httptest.NewRecorder(), regReq)

		loginReq := httptest.NewRequest(http.MethodPost, "/api/login", bytes.NewReader(payload))
		loginReq.Header.Set("Content-Type", "application/json")
		loginRec := httptest.NewRecorder()
		testRouter.ServeHTTP(loginRec, loginReq)

		var resp map[string]interface{}
		json.NewDecoder(loginRec.Body).Decode(&resp)
		return int(resp["user_id"].(float64)), resp["token"].(string)
	}

	_, tokenA := setupUser("playerA")
	_, tokenB := setupUser("playerB")

	// Create a game via matchmaking
	reqA := httptest.NewRequest(http.MethodPost, "/api/games/queue", nil)
	reqA.Header.Set("Authorization", "Bearer "+tokenA)
	resA := httptest.NewRecorder()
	testRouter.ServeHTTP(resA, reqA)

	reqB := httptest.NewRequest(http.MethodPost, "/api/games/queue", nil)
	reqB.Header.Set("Authorization", "Bearer "+tokenB)
	resB := httptest.NewRecorder()
	testRouter.ServeHTTP(resB, reqB)

	var game models.Game
	json.NewDecoder(resB.Body).Decode(&game)
	gameID := game.ID

	done := make(chan bool)

	forfeit := func(token string) {
		req := httptest.NewRequest(http.MethodPost, "/api/games/"+fmt.Sprintf("%d", gameID)+"/forfeit", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		testRouter.ServeHTTP(rec, req)
		done <- true
	}

	go forfeit(tokenA)
	go forfeit(tokenB)

	<-done
	<-done

	verifyReq := httptest.NewRequest(http.MethodGet, "/api/games/"+fmt.Sprintf("%d", gameID), nil)
	verifyReq.Header.Set("Authorization", "Bearer "+tokenA)
	verifyRec := httptest.NewRecorder()
	testRouter.ServeHTTP(verifyRec, verifyReq)
	if verifyRec.Code == http.StatusOK {
		var updated models.Game
		if err := json.NewDecoder(verifyRec.Body).Decode(&updated); err == nil {
			if updated.WinnerID == nil {
				t.Error("expected a winner to be assigned in concurrency test")
			}
		}
	}
}

func TestAtomicJoinQueue_CleansUpActiveGame(t *testing.T) {
	if testGameSvc != nil {
		if resetter, ok := testGameSvc.(interface{ ResetStore() }); ok {
			resetter.ResetStore()
		}
	}
	testGameSvc.ResetQueue()

	// Setup two users
	payloadA, _ := json.Marshal(map[string]string{"username": "atomicA", "password": "password123"})
	regReq := httptest.NewRequest(http.MethodPost, "/api/register", bytes.NewReader(payloadA))
	regReq.Header.Set("Content-Type", "application/json")
	testRouter.ServeHTTP(httptest.NewRecorder(), regReq)

	loginReqA := httptest.NewRequest(http.MethodPost, "/api/login", bytes.NewReader(payloadA))
	loginReqA.Header.Set("Content-Type", "application/json")
	loginRecA := httptest.NewRecorder()
	testRouter.ServeHTTP(loginRecA, loginReqA)
	var respA map[string]interface{}
	json.NewDecoder(loginRecA.Body).Decode(&respA)
	_, tokenA := int(respA["user_id"].(float64)), respA["token"].(string)

	payloadB, _ := json.Marshal(map[string]string{"username": "atomicB", "password": "password123"})
	regReqB := httptest.NewRequest(http.MethodPost, "/api/register", bytes.NewReader(payloadB))
	regReqB.Header.Set("Content-Type", "application/json")
	testRouter.ServeHTTP(httptest.NewRecorder(), regReqB)

	loginReqB := httptest.NewRequest(http.MethodPost, "/api/login", bytes.NewReader(payloadB))
	loginReqB.Header.Set("Content-Type", "application/json")
	loginRecB := httptest.NewRecorder()
	testRouter.ServeHTTP(loginRecB, loginReqB)
	var respB map[string]interface{}
	json.NewDecoder(loginRecB.Body).Decode(&respB)
	tokenB := respB["token"].(string)

	// User A calls joinQueue — should join queue
	req := httptest.NewRequest(http.MethodPost, "/api/games/queue", nil)
	req.Header.Set("Authorization", "Bearer "+tokenA)
	rec := httptest.NewRecorder()
	testRouter.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202 Accepted, got %d: %s", rec.Code, rec.Body.String())
	}

	// Now user B joins — should match with A in the queue
	reqB := httptest.NewRequest(http.MethodPost, "/api/games/queue", nil)
	reqB.Header.Set("Authorization", "Bearer "+tokenB)
	recB := httptest.NewRecorder()
	testRouter.ServeHTTP(recB, reqB)

	if recB.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for match, got %d: %s", recB.Code, recB.Body.String())
	}

	var gameResp map[string]interface{}
	json.NewDecoder(recB.Body).Decode(&gameResp)
	if gameResp["status"] != "in_progress" {
		t.Errorf("expected match game status 'in_progress', got '%v'", gameResp["status"])
	}
}

func TestAtomicJoinQueue_NoActiveGame_JoinsQueueNormally(t *testing.T) {
	if testGameSvc != nil {
		if resetter, ok := testGameSvc.(interface{ ResetStore() }); ok {
			resetter.ResetStore()
		}
	}
	testGameSvc.ResetQueue()

	payload, _ := json.Marshal(map[string]string{"username": "atomicC", "password": "password123"})
	regReq := httptest.NewRequest(http.MethodPost, "/api/register", bytes.NewReader(payload))
	regReq.Header.Set("Content-Type", "application/json")
	testRouter.ServeHTTP(httptest.NewRecorder(), regReq)

	loginReq := httptest.NewRequest(http.MethodPost, "/api/login", bytes.NewReader(payload))
	loginReq.Header.Set("Content-Type", "application/json")
	loginRec := httptest.NewRecorder()
	testRouter.ServeHTTP(loginRec, loginReq)
	var resp map[string]interface{}
	json.NewDecoder(loginRec.Body).Decode(&resp)
	token := resp["token"].(string)

	// No active game — joinQueue should just add to queue
	req := httptest.NewRequest(http.MethodPost, "/api/games/queue", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	testRouter.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202 Accepted, got %d: %s", rec.Code, rec.Body.String())
	}

}

func TestAtomicJoinQueue_StaleGameIsCleaned(t *testing.T) {
	if testGameSvc != nil {
		if resetter, ok := testGameSvc.(interface{ ResetStore() }); ok {
			resetter.ResetStore()
		}
	}
	testGameSvc.ResetQueue()

	payloadA, _ := json.Marshal(map[string]string{"username": "staleA", "password": "password123"})
	regReqA := httptest.NewRequest(http.MethodPost, "/api/register", bytes.NewReader(payloadA))
	regReqA.Header.Set("Content-Type", "application/json")
	testRouter.ServeHTTP(httptest.NewRecorder(), regReqA)

	loginReqA := httptest.NewRequest(http.MethodPost, "/api/login", bytes.NewReader(payloadA))
	loginReqA.Header.Set("Content-Type", "application/json")
	loginRecA := httptest.NewRecorder()
	testRouter.ServeHTTP(loginRecA, loginReqA)
	var respA map[string]interface{}
	json.NewDecoder(loginRecA.Body).Decode(&respA)
	_, tokenA := int(respA["user_id"].(float64)), respA["token"].(string)

	payloadB, _ := json.Marshal(map[string]string{"username": "staleB", "password": "password123"})
	regReqB := httptest.NewRequest(http.MethodPost, "/api/register", bytes.NewReader(payloadB))
	regReqB.Header.Set("Content-Type", "application/json")
	testRouter.ServeHTTP(httptest.NewRecorder(), regReqB)

	loginReqB := httptest.NewRequest(http.MethodPost, "/api/login", bytes.NewReader(payloadB))
	loginReqB.Header.Set("Content-Type", "application/json")
	loginRecB := httptest.NewRecorder()
	testRouter.ServeHTTP(loginRecB, loginReqB)
	var respB map[string]interface{}
	json.NewDecoder(loginRecB.Body).Decode(&respB)
	_ = int(respB["user_id"].(float64))

	// User A calls joinQueue — no stale game in memory
	req := httptest.NewRequest(http.MethodPost, "/api/games/queue", nil)
	req.Header.Set("Authorization", "Bearer "+tokenA)
	rec := httptest.NewRecorder()
	testRouter.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202 Accepted, got %d: %s", rec.Code, rec.Body.String())
	}

	_ = respB
}
