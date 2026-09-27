package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequireAuthRejectsInvalidCookies(t *testing.T) {
	tests := []struct {
		name  string
		token string
	}{
		{name: "missing cookie"},
		{name: "malformed token", token: "not-a-session-token"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE
			called := false
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
			})

			request := httptest.NewRequest(http.MethodGet, "/me", nil)
			if tt.token != "" {
				request.AddCookie(&http.Cookie{
					Name:  "session",
					Value: tt.token,
				})
			}
			recorder := httptest.NewRecorder()

			// ACT
			// Invalid cookies must be rejected before accessing the nil pool.
			requireAuth(nil, next).ServeHTTP(recorder, request)

			// ASSERT
			if recorder.Code != http.StatusUnauthorized {
				t.Errorf("expected 401, got %d", recorder.Code)
			}
			if called {
				t.Error("protected handler was called without authentication")
			}
		})
	}
}
