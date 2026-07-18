package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/oauth2"
)

// Stateless sessions (the infrastructure constraint): after Google OIDC login
// we mint an HS256 JWT holding the user's identity and set it as an HttpOnly
// cookie. Every replica shares JWT_SECRET, so ANY pod can validate ANY request
// with zero memory and zero DB reads - OOM-kill an API pod and the user's next
// request just lands on another replica, still logged in. Nothing to replicate,
// nothing to lose. (Trade-off, logged in ADR-007: a session can't be revoked
// server-side before its expiry - acceptable for a tamagotchi.)
//
// The IdP issuer is configurable (OIDC_ISSUER, default Google) so the full
// login flow can be end-to-end tested against a local fake provider without
// real Google credentials.

const (
	sessionCookie = "kp_session"
	stateCookie   = "kp_state"
	sessionTTL    = 7 * 24 * time.Hour
)

type authService struct {
	cfg      *oauth2.Config
	verifier *oidc.IDTokenVerifier
	secret   []byte
}

// SessionUser is what the JWT carries - enough to render the UI and scope
// queries, so steady-state requests never touch the users table.
type SessionUser struct {
	ID      int64  `json:"id"`
	Email   string `json:"email"`
	Name    string `json:"name"`
	Picture string `json:"picture"`
}

type sessionClaims struct {
	Email   string `json:"email"`
	Name    string `json:"name"`
	Picture string `json:"picture"`
	jwt.RegisteredClaims
}

// newAuthService returns (nil, nil) when auth env isn't configured - the API
// then runs with auth disabled instead of crash-looping, so the deployment
// doesn't depend on the Google OAuth client existing yet.
func newAuthService(ctx context.Context) (*authService, error) {
	clientID := os.Getenv("GOOGLE_CLIENT_ID")
	clientSecret := os.Getenv("GOOGLE_CLIENT_SECRET")
	redirectURL := os.Getenv("OAUTH_REDIRECT_URL")
	secret := os.Getenv("JWT_SECRET")
	unset := func(v string) bool { return v == "" || strings.HasPrefix(v, "REPLACE_ME") }
	if unset(clientID) || unset(clientSecret) || unset(redirectURL) || unset(secret) {
		log.Printf("auth disabled: GOOGLE_CLIENT_ID/GOOGLE_CLIENT_SECRET/OAUTH_REDIRECT_URL/JWT_SECRET not (fully) configured")
		return nil, nil
	}

	issuer := envOr("OIDC_ISSUER", "https://accounts.google.com")
	provider, err := oidc.NewProvider(ctx, issuer)
	if err != nil {
		return nil, err
	}
	return &authService{
		cfg: &oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			RedirectURL:  redirectURL,
			Endpoint:     provider.Endpoint(),
			Scopes:       []string{oidc.ScopeOpenID, "email", "profile"},
		},
		verifier: provider.Verifier(&oidc.Config{ClientID: clientID}),
		secret:   []byte(secret),
	}, nil
}

func (s *authService) mintSession(u SessionUser) (string, error) {
	now := time.Now()
	claims := sessionClaims{
		Email:   u.Email,
		Name:    u.Name,
		Picture: u.Picture,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   strconv.FormatInt(u.ID, 10),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(sessionTTL)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.secret)
}

func (s *authService) parseSession(token string) (*SessionUser, error) {
	var claims sessionClaims
	_, err := jwt.ParseWithClaims(token, &claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return s.secret, nil
	})
	if err != nil {
		return nil, err
	}
	id, err := strconv.ParseInt(claims.Subject, 10, 64)
	if err != nil {
		return nil, err
	}
	return &SessionUser{ID: id, Email: claims.Email, Name: claims.Name, Picture: claims.Picture}, nil
}

// sessionUser resolves the caller from the session cookie alone. nil = anonymous.
func (a *app) sessionUser(r *http.Request) *SessionUser {
	if a.auth == nil {
		return nil
	}
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return nil
	}
	u, err := a.auth.parseSession(c.Value)
	if err != nil {
		return nil // expired/garbage cookie = anonymous; browser will re-login
	}
	return u
}

// secureCookies: true when the original request came in over HTTPS (directly
// or via the ingress, which sets X-Forwarded-Proto).
func secureCookies(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

// --- handlers ---

func (a *app) authStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"enabled": a.auth != nil,
		"user":    a.sessionUser(r), // null when anonymous
	})
}

func (a *app) me(w http.ResponseWriter, r *http.Request) {
	u := a.sessionUser(r)
	if u == nil {
		writeError(w, http.StatusUnauthorized, "not logged in")
		return
	}
	writeJSON(w, http.StatusOK, u)
}

func (a *app) authLogin(w http.ResponseWriter, r *http.Request) {
	if a.auth == nil {
		writeError(w, http.StatusServiceUnavailable, "auth not configured")
		return
	}
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		writeError(w, http.StatusInternalServerError, "entropy failure")
		return
	}
	state := hex.EncodeToString(buf)
	http.SetCookie(w, &http.Cookie{
		Name: stateCookie, Value: state, Path: "/", MaxAge: 600,
		HttpOnly: true, Secure: secureCookies(r), SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, a.auth.cfg.AuthCodeURL(state), http.StatusFound)
}

func (a *app) authCallback(w http.ResponseWriter, r *http.Request) {
	if a.auth == nil {
		writeError(w, http.StatusServiceUnavailable, "auth not configured")
		return
	}
	fail := func(code string, err error) {
		log.Printf("auth callback failed (%s): %v", code, err)
		http.Redirect(w, r, "/?auth_error="+code, http.StatusFound)
	}

	stateC, err := r.Cookie(stateCookie)
	if err != nil || stateC.Value == "" || r.URL.Query().Get("state") != stateC.Value {
		fail("state_mismatch", err)
		return
	}
	// state is single-use - clear it
	http.SetCookie(w, &http.Cookie{Name: stateCookie, Value: "", Path: "/", MaxAge: -1})

	token, err := a.auth.cfg.Exchange(r.Context(), r.URL.Query().Get("code"))
	if err != nil {
		fail("exchange", err)
		return
	}
	rawID, ok := token.Extra("id_token").(string)
	if !ok {
		fail("no_id_token", nil)
		return
	}
	idToken, err := a.auth.verifier.Verify(r.Context(), rawID)
	if err != nil {
		fail("verify", err)
		return
	}
	var claims struct {
		Sub     string `json:"sub"`
		Email   string `json:"email"`
		Name    string `json:"name"`
		Picture string `json:"picture"`
	}
	if err := idToken.Claims(&claims); err != nil || claims.Sub == "" {
		fail("claims", err)
		return
	}

	// Upsert the user; identity is keyed on Google's stable subject id.
	var u SessionUser
	err = a.db.QueryRow(r.Context(),
		`INSERT INTO users (google_sub, email, name, picture)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (google_sub)
		 DO UPDATE SET email = $2, name = $3, picture = $4
		 RETURNING id, email, name, picture`,
		claims.Sub, claims.Email, claims.Name, claims.Picture,
	).Scan(&u.ID, &u.Email, &u.Name, &u.Picture)
	if err != nil {
		fail("upsert", err)
		return
	}

	session, err := a.auth.mintSession(u)
	if err != nil {
		fail("mint", err)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: session, Path: "/",
		MaxAge: int(sessionTTL.Seconds()), HttpOnly: true,
		Secure: secureCookies(r), SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, "/", http.StatusFound)
}

func (a *app) authLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: secureCookies(r), SameSite: http.SameSiteLaxMode,
	})
	w.WriteHeader(http.StatusNoContent)
}
