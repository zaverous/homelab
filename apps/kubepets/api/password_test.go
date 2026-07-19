package main

import "testing"

func TestHashPasswordRoundTrip(t *testing.T) {
	hash, err := hashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("hashPassword: %v", err)
	}
	if !verifyPassword("correct horse battery staple", hash) {
		t.Fatal("correct password did not verify")
	}
	if verifyPassword("wrong password", hash) {
		t.Fatal("wrong password verified")
	}
}

func TestHashPasswordIsSalted(t *testing.T) {
	// Same input must yield different encodings (random per-password salt),
	// yet both must verify.
	a, err := hashPassword("hunter2hunter2")
	if err != nil {
		t.Fatalf("hashPassword a: %v", err)
	}
	b, err := hashPassword("hunter2hunter2")
	if err != nil {
		t.Fatalf("hashPassword b: %v", err)
	}
	if a == b {
		t.Fatal("expected distinct hashes for the same password (missing salt?)")
	}
	if !verifyPassword("hunter2hunter2", a) || !verifyPassword("hunter2hunter2", b) {
		t.Fatal("salted hashes failed to verify")
	}
}

func TestVerifyPasswordRejectsMalformed(t *testing.T) {
	for _, enc := range []string{
		"",
		"not-a-hash",
		"bcrypt$12$salt$hash",         // wrong algorithm tag
		"pbkdf2-sha256$abc$c2FsdA$aGFzaA", // non-numeric iteration count
		"pbkdf2-sha256$1000$only-three",   // too few fields
	} {
		if verifyPassword("whatever", enc) {
			t.Fatalf("malformed encoding verified as valid: %q", enc)
		}
	}
}

func TestValidEmail(t *testing.T) {
	good := []string{"a@b.com", "keeper.of.pets@zaverous.com", "x+tag@sub.example.org"}
	bad := []string{"", "not-an-email", "a@b@c.com", "Name <a@b.com>", "a@b.com "}
	for _, e := range good {
		if !validEmail(e) {
			t.Errorf("expected %q to be valid", e)
		}
	}
	for _, e := range bad {
		if validEmail(e) {
			t.Errorf("expected %q to be invalid", e)
		}
	}
}

func TestHashTokenIsDeterministic(t *testing.T) {
	if hashToken("abc123") != hashToken("abc123") {
		t.Fatal("hashToken not deterministic")
	}
	if hashToken("abc123") == hashToken("abc124") {
		t.Fatal("hashToken collided on different input")
	}
}
