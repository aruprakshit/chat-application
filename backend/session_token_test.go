package main

import (
	"bytes"
	"encoding/base64"
	"testing"
)

func TestSessionTokenRoundTrip(t *testing.T) {
	// ARRANGE / ACT: Generate a browser token and its storage hash.
	token, storedHash := newSessionToken()

	// ASSERT: The token encodes the expected amount of random data.
	decoded, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		t.Fatal("token is not valid URL-safe base64")
	}
	if len(decoded) != 32 {
		t.Fatalf("expected 32 random bytes, got %d", len(decoded))
	}

	// Simulate receiving the token back from the browser.
	lookupHash := hashSessionToken(token)

	if len(storedHash) != 32 {
		t.Fatalf("expected 32-byte hash, got %d", len(storedHash))
	}
	if !bytes.Equal(lookupHash, storedHash) {
		t.Fatal("returned token does not match its stored hash")
	}

	// An altered token must not resolve to the original session hash.
	alteredHash := hashSessionToken(token + "x")
	if bytes.Equal(alteredHash, storedHash) {
		t.Fatal("altered token matched the original session hash")
	}
}
