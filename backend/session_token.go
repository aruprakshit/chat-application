package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
)

// newSessionToken returns:
// - the secret token for the browser
// - its SHA-256 hash for database storage
func newSessionToken() (string, []byte) {
	// 1. GENERATE RANDOM BYTES
	// 32 bytes provide 256 bits of cryptographic randomness.
	randomBytes := make([]byte, 32)
	rand.Read(randomBytes)

	// 2. ENCODE FOR A COOKIE
	// Base64 converts arbitrary bytes into cookie-safe text.
	// Encoding is not encryption and adds no extra randomness.
	token := base64.RawURLEncoding.EncodeToString(randomBytes)

	// 3. HASH THE EXACT TOKEN THE BROWSER WILL SEND BACK
	return token, hashSessionToken(token)
}

func hashSessionToken(token string) []byte {
	// SHA-256 always produces 32 bytes.
	// The same token always produces the same hash.
	hash := sha256.Sum256([]byte(token))

	// Convert the fixed-size array into a byte slice for the DB driver.
	return hash[:]
}
