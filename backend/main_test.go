package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthHandler(t *testing.T) {
	// ARRANGE: Prepare the request and response recorder.
	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	recorder := httptest.NewRecorder()

	// ACT: Run the behavior we are testing.
	healthHandler(recorder, request)

	// ASSERT: Check the resulting response.
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recorder.Code)
	}

	if got := recorder.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("expected application/json, got %q", got)
	}

	var response HealthResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}

	if response.Status != "ok" {
		t.Errorf("expected status ok, got %q", response.Status)
	}

	if response.Service != "chat-backend" {
		t.Errorf("expected service chat-backend, got %q", response.Service)
	}
}

func TestRouterRejectsPostHealth(t *testing.T) {
	// ARRANGE
	router := newRouter()
	request := httptest.NewRequest(http.MethodPost, "/health", nil)
	recorder := httptest.NewRecorder()

	// ACT: Send the request through the router.
	router.ServeHTTP(recorder, request)

	// ASSERT
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", recorder.Code)
	}
}