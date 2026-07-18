// KubePets API - wiring only; features live in their own files:
//   migrate.go  schema (users + pets.owner_id)
//   auth.go     Google OIDC login -> stateless JWT session cookie
//   pets.go     pet CRUD, owner-scoped
//   chaos.go    /chaos/batch-feed - floods the Redis queue for HPA/OOM demos
//
// Statelessness is a hard requirement (the platform team OOM-kills API pods
// on purpose): no in-memory sessions, no sticky anything. Every request is
// authenticated from the JWT cookie alone; any replica can serve any request.
package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type app struct {
	db       *pgxpool.Pool
	rdb      *redis.Client
	auth     *authService // nil => auth disabled (env not configured)
	queueKey string
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func (a *app) healthz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	if err := a.db.Ping(ctx); err != nil {
		writeError(w, http.StatusServiceUnavailable, "db unreachable")
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func scanPet(row pgx.Row) (Pet, error) {
	var p Pet
	err := row.Scan(&p.ID, &p.Name, &p.Hunger, &p.CreatedAt, &p.LastFedAt)
	return p, err
}

func main() {
	ctx := context.Background()

	db, err := pgxpool.New(ctx, "")
	if err != nil {
		log.Fatalf("db config: %v", err)
	}
	defer db.Close()

	if err := migrate(ctx, db); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	rdb := redis.NewClient(&redis.Options{
		Addr: envOr("REDIS_ADDR", "redis.kubepets.svc.cluster.local:6379"),
	})
	defer rdb.Close()
	// Redis being down must not take the pet API down with it - only the
	// chaos endpoint needs it, and that fails per-request with a clear error.
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Printf("warning: redis unreachable at startup (chaos endpoint degraded): %v", err)
	}

	auth, err := newAuthService(ctx)
	if err != nil {
		log.Fatalf("auth init: %v", err)
	}

	a := &app{db: db, rdb: rdb, auth: auth, queueKey: envOr("QUEUE_KEY", "hunger-queue")}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /pets", a.createPet)
	mux.HandleFunc("GET /pets", a.listPets)
	mux.HandleFunc("GET /pets/{id}", a.getPet)
	mux.HandleFunc("POST /pets/{id}/feed", a.feedPet)
	mux.HandleFunc("GET /auth/status", a.authStatus)
	mux.HandleFunc("GET /auth/login", a.authLogin)
	mux.HandleFunc("GET /auth/callback", a.authCallback)
	mux.HandleFunc("POST /auth/logout", a.authLogout)
	mux.HandleFunc("GET /me", a.me)
	mux.HandleFunc("POST /chaos/batch-feed", a.batchFeed)
	mux.HandleFunc("GET /healthz", a.healthz)

	addr := envOr("ADDR", ":8080")
	log.Printf("kubepets-api listening on %s (auth: %v)", addr, auth != nil)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}
