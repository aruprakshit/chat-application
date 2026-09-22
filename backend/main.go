package main

import (
	"encoding/json"
	"log"
	"net/http"
)

type HealthResponse struct {
	Status  string `json:"status"`
	Service string `json:"service"`
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

func main() {
	http.HandleFunc("GET /health", healthHandler)

	log.Println("Server listening on port 8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
