package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCreateUserRejectsInvalidInput(t *testing.T) {
	// 1. DEFINE THE CASES
	// Each case describes one request and its expected HTTP status.
	tests := []struct {
		name        string
		contentType string
		body        string
		wantStatus  int
	}{
		{
			name:        "unsupported content type",
			contentType: "text/plain",
			body:        `{"username":"bob"}`,
			wantStatus:  http.StatusUnsupportedMediaType,
		},
		{
			name:        "malformed JSON",
			contentType: "application/json",
			body:        `{"username":`,
			wantStatus:  http.StatusBadRequest,
		},
		{
			name:        "missing username",
			contentType: "application/json",
			body:        `{}`,
			wantStatus:  http.StatusBadRequest,
		},
		{
			name:        "username too short",
			contentType: "application/json",
			body:        `{"username":"ab"}`,
			wantStatus:  http.StatusBadRequest,
		},
		{
			name:        "uppercase username",
			contentType: "application/json",
			body:        `{"username":"Alice"}`,
			wantStatus:  http.StatusBadRequest,
		},
		{
			name:        "unknown field",
			contentType: "application/json",
			body:        `{"username":"bob", "password":"test-only-password-123", "admin":true}`,
			wantStatus:  http.StatusBadRequest,
		},
		{
			name:        "multiple JSON values",
			contentType: "application/json",
			body:        `{"username":"bob"} {"username":"alice"}`,
			wantStatus:  http.StatusBadRequest,
		},
		{
			name:        "oversized body",
			contentType: "application/json",
			body:        `{"username":"` + strings.Repeat("a", 1100) + `"}`,
			wantStatus:  http.StatusBadRequest,
		},
		{
			name:        "missing password",
			contentType: "application/json",
			body:        `{"username":"bob"}`,
			wantStatus:  http.StatusBadRequest,
		},
		{
			name:        "short password",
			contentType: "application/json",
			body:        `{"username":"bob","password":"short"}`,
			wantStatus:  http.StatusBadRequest,
		},
		{
			name:        "password exceeds bcrypt limit",
			contentType: "application/json",
			body:        `{"username":"bob","password":"` + strings.Repeat("a", 73) + `"}`,
			wantStatus:  http.StatusBadRequest,
		},
	}

	// 2. RUN EACH CASE AS A NAMED SUBTEST
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE: Build a request with this case's input.
			request := httptest.NewRequest(
				http.MethodPost,
				"/users",
				strings.NewReader(tt.body),
			)
			request.Header.Set("Content-Type", tt.contentType)
			recorder := httptest.NewRecorder()

			// No database is needed: invalid input must be rejected
			// before the handler attempts a database operation.
			handler := createUserHandler(nil)

			// ACT: Run the handler.
			handler.ServeHTTP(recorder, request)

			// ASSERT: Check the response status.
			if recorder.Code != tt.wantStatus {
				t.Errorf(
					"expected status %d, got %d; body=%q",
					tt.wantStatus,
					recorder.Code,
					recorder.Body.String(),
				)
			}
		})
	}
}
