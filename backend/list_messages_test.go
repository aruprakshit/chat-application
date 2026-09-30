package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListMessagesRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name           string
		conversationID string
		query          string
	}{
		{"invalid conversation ID", "abc", ""},
		{"zero conversation ID", "0", ""},
		{"negative conversation ID", "-1", ""},
		{"empty limit", "1", "?limit="},
		{"noninteger limit", "1", "?limit=abc"},
		{"zero limit", "1", "?limit=0"},
		{"negative limit", "1", "?limit=-1"},
		{"limit exceeds maximum", "1", "?limit=101"},
		{"repeated limit", "1", "?limit=1&limit=2"},
		{"empty cursor", "1", "?before_id="},
		{"noninteger cursor", "1", "?before_id=abc"},
		{"zero cursor", "1", "?before_id=0"},
		{"negative cursor", "1", "?before_id=-1"},
		{"cursor overflows int64", "1", "?before_id=9223372036854775808"},
		{"repeated cursor", "1", "?before_id=5&before_id=3"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE: Simulate a request from an authenticated user.
			request := httptest.NewRequest(
				http.MethodGet,
				"/conversations/"+tt.conversationID+"/messages"+tt.query,
				nil,
			)
			request.SetPathValue("id", tt.conversationID)

			ctx := context.WithValue(
				request.Context(),
				authContextKey{},
				User{ID: 6, Username: "test_reader"},
			)
			request = request.WithContext(ctx)

			recorder := httptest.NewRecorder()

			// ACT: Invalid input must return before using the nil pool.
			listMessagesHandler(nil).ServeHTTP(recorder, request)

			// ASSERT: Invalid pagination or path input receives 400.
			if recorder.Code != http.StatusBadRequest {
				t.Errorf("expected 400, got %d; body=%q",
					recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestListMessagesRequiresIdentity(t *testing.T) {
	// ARRANGE: Valid path, but no authenticated user in the context.
	request := httptest.NewRequest(
		http.MethodGet,
		"/conversations/1/messages",
		nil,
	)
	request.SetPathValue("id", "1")
	recorder := httptest.NewRecorder()

	// ACT
	listMessagesHandler(nil).ServeHTTP(recorder, request)

	// ASSERT
	if recorder.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d; body=%q",
			recorder.Code, recorder.Body.String())
	}
}
