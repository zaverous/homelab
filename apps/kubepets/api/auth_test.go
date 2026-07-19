package main

import (
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
