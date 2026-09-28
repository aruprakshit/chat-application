package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCreateConversationRejectsInvalidInput(t *testing.T) {
	// 1. DEFINE INVALID REQUESTS AND THEIR EXPECTED RESPONSES
	tests := []struct {
		name        string
		contentType string
		body        string
		wantStatus  int
	}{
		{
			name:        "unsupported content type",
			contentType: "text/plain",
			body:        `{"participant_id":5}`,
			wantStatus:  http.StatusUnsupportedMediaType,
		},
		{
			name:        "malformed JSON",
			contentType: "application/json",
			body:        `{"participant_id":`,
			wantStatus:  http.StatusBadRequest,
		},
		{
			name:        "missing participant",
			contentType: "application/json",
			body:        `{}`,
			wantStatus:  http.StatusBadRequest,
		},
		{
			name:        "null participant",
			contentType: "application/json",
			body:        `{"participant_id":null}`,
			wantStatus:  http.StatusBadRequest,
		},
		{
			name:        "zero participant",
			contentType: "application/json",
			body:        `{"participant_id":0}`,
			wantStatus:  http.StatusBadRequest,
		},
		{
			name:        "negative participant",
			contentType: "application/json",
			body:        `{"participant_id":-1}`,
			wantStatus:  http.StatusBadRequest,
		},
		{
			name:        "self participant",
			contentType: "application/json",
			body:        `{"participant_id":6}`,
			wantStatus:  http.StatusBadRequest,
		},
		{
			name:        "participant must be an integer",
			contentType: "application/json",
			body:        `{"participant_id":5.5}`,
			wantStatus:  http.StatusBadRequest,
		},
		{
			name:        "client cannot choose the creator",
			contentType: "application/json",
			body:        `{"participant_id":5,"creator_id":99}`,
			wantStatus:  http.StatusBadRequest,
		},
		{
			name:        "multiple JSON values",
			contentType: "application/json",
			body:        `{"participant_id":5} {"participant_id":7}`,
			wantStatus:  http.StatusBadRequest,
		},
		{
			name:        "oversized body",
			contentType: "application/json",
			body:        `{"participant_id":5}` + strings.Repeat(" ", 1024),
			wantStatus:  http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE: Simulate an identity already verified by middleware.
			// ID 6 is just a test value; no real user record is needed.
			request := httptest.NewRequest(
				http.MethodPost,
				"/conversations",
				strings.NewReader(tt.body),
			)
			request.Header.Set("Content-Type", tt.contentType)

			ctx := context.WithValue(
				request.Context(),
				authContextKey{},
				User{ID: 6, Username: "test_creator"},
			)
			request = request.WithContext(ctx)

			recorder := httptest.NewRecorder()

			// ACT: Invalid input must return before using the nil pool.
			createConversationHandler(nil).ServeHTTP(recorder, request)

			// ASSERT
			if recorder.Code != tt.wantStatus {
				t.Errorf("expected %d, got %d; body=%q",
					tt.wantStatus,
					recorder.Code,
					recorder.Body.String(),
				)
			}
		})
	}
}

func TestCreateConversationRequiresIdentity(t *testing.T) {
	// ARRANGE: No authenticated user is attached to this request.
	request := httptest.NewRequest(
		http.MethodPost,
		"/conversations",
		strings.NewReader(`{"participant_id":5}`),
	)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	// ACT
	createConversationHandler(nil).ServeHTTP(recorder, request)

	// ASSERT
	if recorder.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", recorder.Code)
	}
}
