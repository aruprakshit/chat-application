package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestConversationSocketRouteRejectsUnauthorizedRequests(t *testing.T) {
	const allowedOrigin = "http://localhost:3005"

	// ARRANGE: Configure the router without requiring Compose settings.
	// t.Setenv restores the previous environment value after this test.
	t.Setenv("WS_ALLOWED_ORIGIN", allowedOrigin)
	router := newRouter(nil)

	tests := []struct {
		name       string
		origin     string
		token      string
		wantStatus int
	}{
		{
			name:       "missing origin rejected before authentication",
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "untrusted origin rejected before authentication",
			origin:     "https://untrusted.example",
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "allowed origin but missing cookie",
			origin:     allowedOrigin,
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "allowed origin but malformed cookie",
			origin:     allowedOrigin,
			token:      "invalid-token",
			wantStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE
			request := httptest.NewRequest(
				http.MethodGet, "/ws/conversations/1", nil,
			)
			if tt.origin != "" {
				request.Header.Set("Origin", tt.origin)
			}
			if tt.token != "" {
				request.AddCookie(&http.Cookie{
					Name:  "session",
					Value: tt.token,
				})
			}
			recorder := httptest.NewRecorder()

			// ACT: Exercise the actual route and middleware ordering.
			router.ServeHTTP(recorder, request)

			// ASSERT: Rejection must happen before using the nil pool.
			if recorder.Code != tt.wantStatus {
				t.Errorf("expected %d, got %d; body=%q",
					tt.wantStatus, recorder.Code, recorder.Body.String())
			}
		})
	}
}
