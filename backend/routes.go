package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
)

type HealthResponse struct {
	Status  string `json:"status"`
	Service string `json:"service"`
}

// protected wraps a handler with cross-origin protection and authentication.
// Use it for any state-changing route that must reject cross-site requests.
func protected(pool *pgxpool.Pool, handler http.Handler) http.Handler {
	return http.NewCrossOriginProtection().Handler(
		requireAuth(pool, handler),
	)
}

func newRouter(pool *pgxpool.Pool) http.Handler {
	// Default to HTTPS-only cookies.
	// Disable explicitly only for local HTTP development.
	secureCookie := os.Getenv("COOKIE_SECURE") != "false"

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", healthHandler)
	mux.HandleFunc("GET /ready", readinessHandler(pool))
	mux.HandleFunc("GET /users", usersHandler(pool))
	mux.HandleFunc("POST /users", createUserHandler(pool))
	mux.Handle("GET /me", requireAuth(pool, http.HandlerFunc(meHandler)))
	mux.HandleFunc("POST /login", loginHandler(pool, secureCookie))
	mux.Handle("GET /conversations", requireAuth(pool, conversationsHandler(pool)))
	mux.Handle(
		"POST /conversations",
		protected(pool, createConversationHandler(pool)),
	)
	mux.Handle(
		"POST /conversations/{id}/messages",
		protected(pool, createMessageHandler(pool)),
	)
	// Block cross-origin browser requests that could log a user out.
	mux.Handle(
		"POST /logout",
		http.NewCrossOriginProtection().Handler(
			logoutHandler(pool, secureCookie),
		),
	)
	mux.Handle(
		"GET /conversations/{id}/messages",
		requireAuth(pool, listMessagesHandler(pool)),
	)
	mux.Handle(
		"GET /ws/conversations/{id}",
		requireWebSocketOrigin(
			os.Getenv("WS_ALLOWED_ORIGIN"),
			requireAuth(pool, conversationSocketHandler(pool)),
		),
	)
	return mux
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	response := HealthResponse{
		Status:  "ok",
		Service: "chat-backend",
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Failed to write health response: %v", err)
	}
}
