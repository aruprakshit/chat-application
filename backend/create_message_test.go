package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCreateMessageRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name           string
		conversationID string
		contentType    string
		body           string
		wantStatus     int
	}{
		{
			name:           "invalid conversation ID",
			conversationID: "abc",
			contentType:    "application/json",
			body:           `{"body":"Hello"}`,
			wantStatus:     http.StatusBadRequest,
		},
		{
			name:           "zero conversation ID",
			conversationID: "0",
			contentType:    "application/json",
			body:           `{"body":"Hello"}`,
			wantStatus:     http.StatusBadRequest,
		},
		{
			name:           "negative conversation ID",
			conversationID: "-1",
			contentType:    "application/json",
			body:           `{"body":"Hello"}`,
			wantStatus:     http.StatusBadRequest,
		},
		{
			name:           "unsupported content type",
			conversationID: "1",
			contentType:    "text/plain",
			body:           `{"body":"Hello"}`,
			wantStatus:     http.StatusUnsupportedMediaType,
		},
		{
			name:           "malformed JSON",
			conversationID: "1",
			contentType:    "application/json",
			body:           `{"body":`,
			wantStatus:     http.StatusBadRequest,
		},
		{
			name:           "missing body",
			conversationID: "1",
			contentType:    "application/json",
			body:           `{}`,
			wantStatus:     http.StatusBadRequest,
		},
		{
			name:           "null body",
			conversationID: "1",
			contentType:    "application/json",
			body:           `{"body":null}`,
			wantStatus:     http.StatusBadRequest,
		},
		{
			name:           "body must be a string",
			conversationID: "1",
			contentType:    "application/json",
			body:           `{"body":123}`,
			wantStatus:     http.StatusBadRequest,
		},
		{
			name:           "empty body",
			conversationID: "1",
			contentType:    "application/json",
			body:           `{"body":""}`,
			wantStatus:     http.StatusBadRequest,
		},
		{
			name:           "whitespace only",
			conversationID: "1",
			contentType:    "application/json",
			body:           `{"body":" \t\n\u2003"}`,
			wantStatus:     http.StatusBadRequest,
		},
		{
			name:           "more than 4000 characters",
			conversationID: "1",
			contentType:    "application/json",
			body:           `{"body":"` + strings.Repeat("界", 4001) + `"}`,
			wantStatus:     http.StatusBadRequest,
		},
		{
			name:           "client cannot choose sender",
			conversationID: "1",
			contentType:    "application/json",
			body:           `{"body":"Hello","sender_id":99}`,
			wantStatus:     http.StatusBadRequest,
		},
		{
			name:           "multiple JSON values",
			conversationID: "1",
			contentType:    "application/json",
			body:           `{"body":"Hello"} {"body":"Again"}`,
			wantStatus:     http.StatusBadRequest,
		},
		{
			name:           "request exceeds byte limit",
			conversationID: "1",
			contentType:    "application/json",
			body:           `{"body":"Hello"}` + strings.Repeat(" ", 32<<10),
			wantStatus:     http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE: Build the request and simulate a verified identity.
			request := httptest.NewRequest(
				http.MethodPost,
				"/conversations/"+tt.conversationID+"/messages",
				strings.NewReader(tt.body),
			)
			request.Header.Set("Content-Type", tt.contentType)

			// We call the handler directly, so no router populates {id}.
			request.SetPathValue("id", tt.conversationID)

			ctx := context.WithValue(
				request.Context(),
				authContextKey{},
				User{ID: 6, Username: "test_sender"},
			)
			request = request.WithContext(ctx)

			recorder := httptest.NewRecorder()

			// ACT: Invalid input must return before accessing the nil pool.
			createMessageHandler(nil).ServeHTTP(recorder, request)

			// ASSERT: Check the HTTP response.
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

func TestCreateMessageRequiresIdentity(t *testing.T) {
	// ARRANGE: A valid request without an authenticated identity.
	request := httptest.NewRequest(
		http.MethodPost,
		"/conversations/1/messages",
		strings.NewReader(`{"body":"Hello"}`),
	)
	request.Header.Set("Content-Type", "application/json")
	request.SetPathValue("id", "1")
	recorder := httptest.NewRecorder()

	// ACT
	createMessageHandler(nil).ServeHTTP(recorder, request)

	// ASSERT
	if recorder.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d; body=%q",
			recorder.Code, recorder.Body.String())
	}
}
