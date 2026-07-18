package main

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Self-migrating schema, same pattern as the original pets table: idempotent
// statements run at startup, so a fresh Postgres and an existing one both end
// up in the same shape without a migration tool.
//
//	users                          pets (additions)
//	  id          SERIAL PK         owner_id INT NULL -> users.id
//	  google_sub  TEXT UNIQUE         NULL = "stray" (pre-auth pets, loadgen
//	  email       TEXT                seeds, anonymous visitors)
//	  name        TEXT
//	  picture     TEXT
//	  created_at  TIMESTAMPTZ
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
		`CREATE INDEX IF NOT EXISTS pets_owner_idx ON pets (owner_id)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(ctx, s); err != nil {
			return err
		}
	}
	return nil
}
