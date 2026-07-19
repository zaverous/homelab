package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// Email/password accounts, layered on top of the Google OIDC login in auth.go.
// The session mechanism is identical for both: after we establish who the user
// is, issueSession mints the same stateless HS256 cookie, so the rest of the API
// (requireUser, owner-scoped pets) is completely unaware of *how* they logged in.
//
// Anti-enumeration is deliberate throughout: register, forgot, and login return
// the same response whether or not the address exists, so the endpoints can't be
// used to probe which emails have accounts.

const (
	verifyTokenTTL = 24 * time.Hour
	resetTokenTTL  = 1 * time.Hour
	minPasswordLen = 8
)

func normalizeEmail(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

// validEmail stays permissive: exactly one well-formed address, no display
// name. Real proof of ownership is the verification email, not a regex.
func validEmail(s string) bool {
	addr, err := mail.ParseAddress(s)
	return err == nil && addr.Address == s
}

// newToken returns a high-entropy token: the raw value goes in the email link,
// only its SHA-256 hash is stored. High-entropy input means a fast hash is fine
// here (unlike passwords, there is nothing to brute-force).
func newToken() (raw, hashed string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", "", err
	}
	raw = hex.EncodeToString(b)
	return raw, hashToken(raw), nil
}

func hashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func (a *app) publicLink(path string) string {
	return strings.TrimRight(a.baseURL, "/") + path
}

// issueSession mints the stateless JWT and sets it as the session cookie. Shared
// by Google callback, password login, and email verification.
func (a *app) issueSession(w http.ResponseWriter, r *http.Request, u SessionUser) error {
	token, err := a.auth.mintSession(u)
	if err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: token, Path: "/",
		MaxAge: int(sessionTTL.Seconds()), HttpOnly: true,
		Secure: secureCookies(r), SameSite: http.SameSiteLaxMode,
	})
	return nil
}

// --- registration -------------------------------------------------------------

func (a *app) authRegister(w http.ResponseWriter, r *http.Request) {
	if a.auth == nil {
		writeError(w, http.StatusServiceUnavailable, "authentication is not configured")
		return
	}
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		Name     string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	email := normalizeEmail(body.Email)
	if !validEmail(email) {
		writeError(w, http.StatusBadRequest, "a valid email is required")
		return
	}
	if len(body.Password) < minPasswordLen {
		writeError(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		name = strings.SplitN(email, "@", 2)[0]
	}
	hash, err := hashPassword(body.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not secure password")
		return
	}

	const ok = "check your email to verify your account"
	var userID int64
	err = a.db.QueryRow(r.Context(),
		`INSERT INTO users (email, name, password_hash, email_verified)
		 VALUES ($1, $2, $3, false)
		 ON CONFLICT (lower(email)) DO NOTHING
		 RETURNING id`,
		email, name, hash,
	).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		// Address already taken. Don't say so (enumeration); respond exactly as
		// on success. If it's an unverified account, quietly resend the link so
		// a genuine user who re-registers still gets unstuck.
		a.maybeResendVerify(r.Context(), email)
		writeJSON(w, http.StatusOK, map[string]string{"status": ok})
		return
	}
	if err != nil {
		log.Printf("authRegister insert: %v", err)
		writeError(w, http.StatusInternalServerError, "could not create account")
		return
	}
	a.issueVerifyToken(r.Context(), userID, email, name)
	writeJSON(w, http.StatusCreated, map[string]string{"status": ok})
}

// --- password login -----------------------------------------------------------

func (a *app) authPasswordLogin(w http.ResponseWriter, r *http.Request) {
	if a.auth == nil {
		writeError(w, http.StatusServiceUnavailable, "authentication is not configured")
		return
	}
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	email := normalizeEmail(body.Email)

	var u SessionUser
	var hash string
	var verified bool
	err := a.db.QueryRow(r.Context(),
		`SELECT id, email, name, picture, coalesce(password_hash, ''), email_verified
		 FROM users WHERE lower(email) = $1`, email,
	).Scan(&u.ID, &u.Email, &u.Name, &u.Picture, &hash, &verified)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		log.Printf("authPasswordLogin lookup: %v", err)
		writeError(w, http.StatusInternalServerError, "login failed")
		return
	}
	// One uniform failure whether the email is unknown, is a Google-only account
	// (empty hash), or the password is wrong - no oracle. verifyPassword on an
	// empty hash returns false, so the no-row path lands here too.
	if !verifyPassword(body.Password, hash) {
		writeError(w, http.StatusUnauthorized, "invalid email or password")
		return
	}
	if !verified {
		writeError(w, http.StatusForbidden, "verify your email before logging in")
		return
	}
	if err := a.issueSession(w, r, u); err != nil {
		log.Printf("authPasswordLogin session: %v", err)
		writeError(w, http.StatusInternalServerError, "login failed")
		return
	}
	writeJSON(w, http.StatusOK, u)
}

// --- email verification -------------------------------------------------------

func (a *app) authVerify(w http.ResponseWriter, r *http.Request) {
	// A GET, because it's opened straight from an email link. On success it logs
	// the user in and bounces to the SPA - one click, verified and signed in.
	redirect := func(q string) { http.Redirect(w, r, "/"+q, http.StatusFound) }
	if a.auth == nil {
		redirect("?auth_error=verify_unavailable")
		return
	}
	raw := r.URL.Query().Get("token")
	if raw == "" {
		redirect("?auth_error=verify_missing")
		return
	}

	var u SessionUser
	err := a.db.QueryRow(r.Context(),
		`UPDATE users SET email_verified = true
		 WHERE id = (
		   SELECT user_id FROM email_tokens
		   WHERE token_hash = $1 AND kind = 'verify'
		     AND used_at IS NULL AND expires_at > now()
		 )
		 RETURNING id, email, name, picture`,
		hashToken(raw),
	).Scan(&u.ID, &u.Email, &u.Name, &u.Picture)
	if errors.Is(err, pgx.ErrNoRows) {
		redirect("?auth_error=verify_invalid")
		return
	}
	if err != nil {
		log.Printf("authVerify: %v", err)
		redirect("?auth_error=verify_failed")
		return
	}
	// Burn every outstanding verify token for this user (the used one included).
	if _, err := a.db.Exec(r.Context(),
		`UPDATE email_tokens SET used_at = now()
		 WHERE user_id = $1 AND kind = 'verify' AND used_at IS NULL`, u.ID); err != nil {
		log.Printf("authVerify consume tokens: %v", err)
	}
	if err := a.issueSession(w, r, u); err != nil {
		log.Printf("authVerify session: %v", err)
		redirect("?auth_error=verify_session")
		return
	}
	redirect("?verified=1")
}

// authResend re-sends a verification link. Always 200, same anti-enumeration
// stance as register/forgot.
func (a *app) authResend(w http.ResponseWriter, r *http.Request) {
	if a.auth == nil {
		writeError(w, http.StatusServiceUnavailable, "authentication is not configured")
		return
	}
	var body struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if email := normalizeEmail(body.Email); validEmail(email) {
		a.maybeResendVerify(r.Context(), email)
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"status": "if that account still needs verifying, a new link is on its way",
	})
}

// --- password reset -----------------------------------------------------------

func (a *app) authForgot(w http.ResponseWriter, r *http.Request) {
	if a.auth == nil {
		writeError(w, http.StatusServiceUnavailable, "authentication is not configured")
		return
	}
	var body struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if email := normalizeEmail(body.Email); validEmail(email) {
		a.maybeSendReset(r.Context(), email)
	}
	// Always the same answer - never reveal whether the address has an account.
	writeJSON(w, http.StatusOK, map[string]string{
		"status": "if that email has an account, a reset link is on its way",
	})
}

func (a *app) authReset(w http.ResponseWriter, r *http.Request) {
	if a.auth == nil {
		writeError(w, http.StatusServiceUnavailable, "authentication is not configured")
		return
	}
	var body struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if body.Token == "" {
		writeError(w, http.StatusBadRequest, "reset token is required")
		return
	}
	if len(body.Password) < minPasswordLen {
		writeError(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}
	hash, err := hashPassword(body.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not secure password")
		return
	}

	// One statement does it all atomically: find a live reset token, consume it,
	// set the new password, and mark the email verified (using the link proves
	// inbox control). If the token is missing/expired/used, no row updates ->
	// ErrNoRows -> a clean "invalid link".
	var userID int64
	err = a.db.QueryRow(r.Context(),
		`WITH t AS (
		     SELECT id, user_id FROM email_tokens
		     WHERE token_hash = $1 AND kind = 'reset'
		       AND used_at IS NULL AND expires_at > now()
		 ), consumed AS (
		     UPDATE email_tokens SET used_at = now()
		     WHERE id IN (SELECT id FROM t)
		 )
		 UPDATE users SET password_hash = $2, email_verified = true
		 WHERE id = (SELECT user_id FROM t)
		 RETURNING id`,
		hashToken(body.Token), hash,
	).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusBadRequest, "this reset link is invalid or has expired")
		return
	}
	if err != nil {
		log.Printf("authReset: %v", err)
		writeError(w, http.StatusInternalServerError, "could not reset password")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "password updated - you can log in now"})
}

// --- helpers (best-effort; they log and swallow errors so the anti-enumeration
//     responses stay uniform and side-effect-free from the caller's view) ------

func (a *app) issueVerifyToken(ctx context.Context, userID int64, email, name string) {
	raw, hashed, err := newToken()
	if err != nil {
		log.Printf("issueVerifyToken entropy: %v", err)
		return
	}
	if _, err := a.db.Exec(ctx,
		`INSERT INTO email_tokens (user_id, kind, token_hash, expires_at)
		 VALUES ($1, 'verify', $2, $3)`,
		userID, hashed, time.Now().Add(verifyTokenTTL)); err != nil {
		log.Printf("issueVerifyToken insert: %v", err)
		return
	}
	a.sendVerifyEmail(email, name, raw)
}

func (a *app) maybeResendVerify(ctx context.Context, email string) {
	var userID int64
	var name string
	var verified, hasPassword bool
	err := a.db.QueryRow(ctx,
		`SELECT id, name, email_verified, password_hash IS NOT NULL
		 FROM users WHERE lower(email) = $1`, email,
	).Scan(&userID, &name, &verified, &hasPassword)
	if err != nil {
		return // no such user, or a transient error - stay silent either way
	}
	if verified || !hasPassword {
		return // already usable, or a Google-only account (nothing to verify)
	}
	a.issueVerifyToken(ctx, userID, email, name)
}

func (a *app) maybeSendReset(ctx context.Context, email string) {
	var userID int64
	var name, hash string
	err := a.db.QueryRow(ctx,
		`SELECT id, name, coalesce(password_hash, '')
		 FROM users WHERE lower(email) = $1`, email,
	).Scan(&userID, &name, &hash)
	if err != nil {
		return // unknown address - silent
	}
	if hash == "" {
		return // Google-only account: no password to reset
	}
	raw, hashed, err := newToken()
	if err != nil {
		log.Printf("maybeSendReset entropy: %v", err)
		return
	}
	if _, err := a.db.Exec(ctx,
		`INSERT INTO email_tokens (user_id, kind, token_hash, expires_at)
		 VALUES ($1, 'reset', $2, $3)`,
		userID, hashed, time.Now().Add(resetTokenTTL)); err != nil {
		log.Printf("maybeSendReset insert: %v", err)
		return
	}
	a.sendResetEmail(email, name, raw)
}

func (a *app) sendVerifyEmail(email, name, rawToken string) {
	link := a.publicLink("/api/auth/verify?token=" + rawToken)
	if a.mail == nil {
		// Dev fallback: no relay, so print the link the operator (or a local
		// tester) can click. Never happens in prod, where SMTP is configured.
		log.Printf("mail disabled - verify link for %s: %s", email, link)
		return
	}
	body := "Welcome to KubePets" + greetingName(name) + ".\r\n\r\n" +
		"Confirm this address to wake your creatures:\r\n" + link +
		"\r\n\r\nThe link expires in 24 hours. If you didn't sign up, ignore this email.\r\n"
	if err := a.mail.send(email, "Verify your KubePets account", body); err != nil {
		log.Printf("sendVerifyEmail to %s: %v", email, err)
	}
}

func (a *app) sendResetEmail(email, name, rawToken string) {
	link := a.publicLink("/?reset_token=" + rawToken)
	if a.mail == nil {
		log.Printf("mail disabled - reset link for %s: %s", email, link)
		return
	}
	body := "A password reset was requested for your KubePets account" + greetingName(name) + ".\r\n\r\n" +
		"Choose a new password here:\r\n" + link +
		"\r\n\r\nThe link expires in 1 hour. If you didn't request this, ignore this email - your password stays unchanged.\r\n"
	if err := a.mail.send(email, "Reset your KubePets password", body); err != nil {
		log.Printf("sendResetEmail to %s: %v", email, err)
	}
}

func greetingName(name string) string {
	if name = strings.TrimSpace(name); name != "" {
		return ", " + name
	}
	return ""
}

// upsertGoogleUser resolves a Google identity to a single user row, linking to
// an existing email/password account instead of duplicating a person who signed
// up both ways. Three cases, in order:
//
//  1. we've seen this google_sub before -> refresh the profile;
//  2. an email/password account owns this address -> attach google_sub to it
//     (Google has proven the address, so also flip email_verified);
//  3. nobody yet -> create a fresh, already-verified user.
//
// The unique indexes on google_sub and lower(email) are the real integrity
// guard; the tiny race between the SELECTs and the INSERT would at worst fail a
// concurrent duplicate signup, which the caller surfaces as a retryable error.
func (a *app) upsertGoogleUser(ctx context.Context, sub, rawEmail, name, picture string) (SessionUser, error) {
	email := normalizeEmail(rawEmail)
	var u SessionUser

	err := a.db.QueryRow(ctx,
		`UPDATE users SET email = $2, name = $3, picture = $4, email_verified = true
		 WHERE google_sub = $1
		 RETURNING id, email, name, picture`,
		sub, email, name, picture,
	).Scan(&u.ID, &u.Email, &u.Name, &u.Picture)
	if err == nil {
		return u, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return u, err
	}

	err = a.db.QueryRow(ctx,
		`UPDATE users
		 SET google_sub = $1, picture = $4, email_verified = true,
		     name = CASE WHEN name = '' THEN $3 ELSE name END
		 WHERE lower(email) = $2 AND google_sub IS NULL
		 RETURNING id, email, name, picture`,
		sub, email, name, picture,
	).Scan(&u.ID, &u.Email, &u.Name, &u.Picture)
	if err == nil {
		return u, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return u, err
	}

	err = a.db.QueryRow(ctx,
		`INSERT INTO users (google_sub, email, name, picture, email_verified)
		 VALUES ($1, $2, $3, $4, true)
		 RETURNING id, email, name, picture`,
		sub, email, name, picture,
	).Scan(&u.ID, &u.Email, &u.Name, &u.Picture)
	return u, err
}
