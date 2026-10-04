package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequireWebSocketOrigin(t *testing.T) {
	const allowedOrigin = "http://localhost:3005"

	tests := []struct {
		name          string
		configuration string
		origins       []string
		wantAllowed   bool
	}{
		{
			name:          "allowed origin",
			configuration: allowedOrigin,
			origins:       []string{allowedOrigin},
			wantAllowed:   true,
		},
		{
			name:          "missing origin",
			configuration: allowedOrigin,
		},
		{
			name:          "untrusted origin",
			configuration: allowedOrigin,
			origins:       []string{"https://untrusted.example"},
		},
		{
			name:          "wrong port",
			configuration: allowedOrigin,
			origins:       []string{"http://localhost:3000"},
		},
		{
			name:          "wrong scheme",
			configuration: allowedOrigin,
			origins:       []string{"https://localhost:3005"},
		},
		{
			name:          "null origin",
			configuration: allowedOrigin,
			origins:       []string{"null"},
		},
		{
			name:          "multiple origins",
			configuration: allowedOrigin,
			origins:       []string{allowedOrigin, "https://untrusted.example"},
		},
		{
			name:    "missing configuration",
			origins: []string{allowedOrigin},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE: Track whether the guard calls the next handler.
			called := false
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				w.WriteHeader(http.StatusNoContent)
			})

			request := httptest.NewRequest(
				http.MethodGet, "/ws/conversations/1", nil,
			)
			for _, origin := range tt.origins {
				request.Header.Add("Origin", origin)
			}
			recorder := httptest.NewRecorder()

			// ACT
			requireWebSocketOrigin(tt.configuration, next).
				ServeHTTP(recorder, request)

			// ASSERT: Verify both the response and downstream execution.
			wantStatus := http.StatusForbidden
			if tt.wantAllowed {
				wantStatus = http.StatusNoContent
			}

			if recorder.Code != wantStatus {
				t.Errorf("expected %d, got %d",
					wantStatus, recorder.Code)
			}
			if called != tt.wantAllowed {
				t.Errorf("expected next handler called=%t, got %t",
					tt.wantAllowed, called)
			}
		})
	}
}
