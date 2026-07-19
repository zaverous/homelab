package main

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
)

// Password hashing with ZERO third-party dependencies: PBKDF2-HMAC-SHA256,
// which was promoted into the standard library (crypto/pbkdf2) in Go 1.24. That
// matters here because the build has no network to fetch golang.org/x/crypto for
// bcrypt/argon2 - staying in std keeps the image reproducible.
//
// Each password gets a fresh random salt and a high iteration count. The stored
// string is self-describing - "pbkdf2-sha256$<iter>$<salt>$<hash>" - so the cost
// can be raised later and old hashes still verify (verifyPassword reads the iter
// count from the record, it isn't hard-coded on the read path).
const (
	pbkdf2Iters   = 600_000 // OWASP 2023 floor for PBKDF2-HMAC-SHA256
	pbkdf2KeyLen  = 32
	pbkdf2SaltLen = 16
)

func hashPassword(plain string) (string, error) {
	salt := make([]byte, pbkdf2SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	dk, err := pbkdf2.Key(sha256.New, plain, salt, pbkdf2Iters, pbkdf2KeyLen)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("pbkdf2-sha256$%d$%s$%s",
		pbkdf2Iters,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(dk),
	), nil
}

// verifyPassword is constant-time in the final comparison so it leaks no timing
// signal about how many leading bytes matched.
func verifyPassword(plain, encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2-sha256" {
		return false
	}
	iters, err := strconv.Atoi(parts[1])
	if err != nil || iters < 1 {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[2])
	if err != nil {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil {
		return false
	}
	got, err := pbkdf2.Key(sha256.New, plain, salt, iters, len(want))
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(got, want) == 1
}
