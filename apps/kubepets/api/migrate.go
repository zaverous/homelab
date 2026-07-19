package main

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Self-migrating schema, same pattern as the original pets table: idempotent
// statements run at startup, so a fresh Postgres and an existing one both end
// up in the same shape without a migration tool.
//
//	users                              pets (additions)
//	  id             SERIAL PK          owner_id INT NULL -> users.id
//	  google_sub     TEXT UNIQUE NULL     NULL = "stray" (pre-auth pets, loadgen
//	  email          TEXT UNIQUE(lower)   seeds, anonymous visitors)
//	  name           TEXT
//	  picture        TEXT
//	  password_hash  TEXT NULL          email_tokens (verify + reset, hashed)
//	  email_verified BOOL                 id, user_id, kind, token_hash,
//	  created_at     TIMESTAMPTZ          expires_at, used_at, created_at
//
// google_sub is nullable now: Google users have one, email/password users
// don't. email/password login keys on the address instead, so email gains a
// case-insensitive UNIQUE index and becomes the human identity both login
// methods converge on (see upsertGoogleUser for the account-linking).
func migrate(ctx context.Context, db *pgxpool.Pool) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS pets (
			id          SERIAL PRIMARY KEY,
			name        TEXT NOT NULL,
			hunger      INT NOT NULL DEFAULT 0,
			created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
			last_fed_at TIMESTAMPTZ
		)`,
		`CREATE TABLE IF NOT EXISTS users (
			id         SERIAL PRIMARY KEY,
			google_sub TEXT UNIQUE NOT NULL,
			email      TEXT NOT NULL,
			name       TEXT NOT NULL DEFAULT '',
			picture    TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`,
		`ALTER TABLE pets ADD COLUMN IF NOT EXISTS
			owner_id INTEGER REFERENCES users(id) ON DELETE CASCADE`,
		`ALTER TABLE pets ADD COLUMN IF NOT EXISTS
			hunger_updated_at TIMESTAMPTZ NOT NULL DEFAULT now()`,
		`CREATE INDEX IF NOT EXISTS pets_owner_idx ON pets (owner_id)`,
		// --- email/password accounts ---
		// Google users keep google_sub; password users have none. Dropping the
		// NOT NULL lets both share the table (a UNIQUE column still permits many
		// NULLs in Postgres, so password rows don't collide).
		`ALTER TABLE users ALTER COLUMN google_sub DROP NOT NULL`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS password_hash TEXT`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS
			email_verified BOOLEAN NOT NULL DEFAULT false`,
		// email is now a login identity -> unique, case-insensitively. Existing
		// Google rows already hold distinct addresses, so this can't fail on
		// established data.
		`CREATE UNIQUE INDEX IF NOT EXISTS users_email_lower_idx ON users (lower(email))`,
		// One-time tokens for email verification and password reset. Only a
		// SHA-256 hash of the token is stored, never the token itself: a DB leak
		// then can't be replayed as a live link (same reasoning as password_hash).
		`CREATE TABLE IF NOT EXISTS email_tokens (
			id         SERIAL PRIMARY KEY,
			user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			kind       TEXT NOT NULL,          -- 'verify' | 'reset'
			token_hash TEXT NOT NULL,
			expires_at TIMESTAMPTZ NOT NULL,
			used_at    TIMESTAMPTZ,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`,
		`CREATE INDEX IF NOT EXISTS email_tokens_hash_idx ON email_tokens (token_hash)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(ctx, s); err != nil {
			return err
		}
	}
	return nil
}
