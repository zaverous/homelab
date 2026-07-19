package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequireUserWhenAuthIsNotConfigured(t *testing.T) {
	a := &app{}
	req := httptest.NewRequest(http.MethodGet, "/pets", nil)
	res := httptest.NewRecorder()

	if user := a.requireUser(res, req); user != nil {
		t.Fatal("expected no user")
	}
	if res.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected %d, got %d", http.StatusServiceUnavailable, res.Code)
	}
}

// A JWT secret alone (no Google OIDC client) must enable email/password auth:
// status reports enabled=true so the frontend shows the login/register form, but
// google=false so it hides the "continue with Google" button.
func TestAuthStatusEnabledWithoutGoogle(t *testing.T) {
	a := &app{auth: &authService{secret: []byte("a-test-secret-that-is-not-used-in-production")}}
	req := httptest.NewRequest(http.MethodGet, "/auth/status", nil)
	res := httptest.NewRecorder()

	a.authStatus(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected %d, got %d", http.StatusOK, res.Code)
	}
	var body struct {
		Enabled bool `json:"enabled"`
		Google  bool `json:"google"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if !body.Enabled {
		t.Error("expected enabled=true (email/password available) with only JWT_SECRET set")
	}
	if body.Google {
		t.Error("expected google=false when no OIDC client is configured")
	}
}

// The Google-only flow must refuse cleanly when OIDC isn't wired up, rather than
// nil-panicking on a.auth.cfg.
func TestGoogleLoginUnavailableWithoutOIDC(t *testing.T) {
	a := &app{auth: &authService{secret: []byte("a-test-secret-that-is-not-used-in-production")}}
	req := httptest.NewRequest(http.MethodGet, "/auth/login", nil)
	res := httptest.NewRecorder()

	a.authLogin(res, req)

	if res.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected %d, got %d", http.StatusServiceUnavailable, res.Code)
	}
}

func TestRequireUserRejectsMissingSession(t *testing.T) {
	a := &app{auth: &authService{secret: []byte("a-test-secret-that-is-not-used-in-production")}}
	req := httptest.NewRequest(http.MethodGet, "/pets", nil)
	res := httptest.NewRecorder()

	if user := a.requireUser(res, req); user != nil {
		t.Fatal("expected no user")
	}
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("expected %d, got %d", http.StatusUnauthorized, res.Code)
	}
}

func TestRequireUserAcceptsValidSession(t *testing.T) {
	a := &app{auth: &authService{secret: []byte("a-test-secret-that-is-not-used-in-production")}}
	want := SessionUser{ID: 42, Email: "keeper@example.com", Name: "Keeper"}
	token, err := a.auth.mintSession(want)
	if err != nil {
		t.Fatalf("mint session: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/pets", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
	res := httptest.NewRecorder()

	got := a.requireUser(res, req)
	if got == nil {
		t.Fatal("expected authenticated user")
	}
	if got.ID != want.ID || got.Email != want.Email || got.Name != want.Name {
		t.Fatalf("unexpected user: %+v", got)
	}
}
