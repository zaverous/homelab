package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Pet struct {
	ID        int64      `json:"id"`
	Name      string     `json:"name"`
	Hunger    int        `json:"hunger"`
	CreatedAt time.Time  `json:"created_at"`
	LastFedAt *time.Time `json:"last_fed_at,omitempty"`
}

var db *pgxpool.Pool

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func scanPet(row pgx.Row) (Pet, error) {
	var p Pet
	err := row.Scan(&p.ID, &p.Name, &p.Hunger, &p.CreatedAt, &p.LastFedAt)
	return p, err
}

func createPet(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	row := db.QueryRow(r.Context(),
		`INSERT INTO pets (name) VALUES ($1)
		 RETURNING id, name, hunger, created_at, last_fed_at`, body.Name)
	pet, err := scanPet(row)
	if err != nil {
		log.Printf("createPet: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to create pet")
		return
	}
	writeJSON(w, http.StatusCreated, pet)
}

func listPets(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query(r.Context(),
		`SELECT id, name, hunger, created_at, last_fed_at FROM pets ORDER BY id`)
	if err != nil {
		log.Printf("listPets: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to list pets")
		return
	}
	defer rows.Close()

	pets := []Pet{}
	for rows.Next() {
		p, err := scanPet(rows)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to read pets")
			return
		}
		pets = append(pets, p)
	}
	writeJSON(w, http.StatusOK, pets)
}

func getPet(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid pet id")
		return
	}
	row := db.QueryRow(r.Context(),
		`SELECT id, name, hunger, created_at, last_fed_at FROM pets WHERE id = $1`, id)
	pet, err := scanPet(row)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "pet not found")
		return
	}
	if err != nil {
		log.Printf("getPet: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to get pet")
		return
	}
	writeJSON(w, http.StatusOK, pet)
}

func feedPet(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid pet id")
		return
	}
	row := db.QueryRow(r.Context(),
		`UPDATE pets SET hunger = 0, last_fed_at = now()
		 WHERE id = $1
		 RETURNING id, name, hunger, created_at, last_fed_at`, id)
	pet, err := scanPet(row)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "pet not found")
		return
	}
	if err != nil {
		log.Printf("feedPet: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to feed pet")
		return
	}
	writeJSON(w, http.StatusOK, pet)
}

func healthz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	if err := db.Ping(ctx); err != nil {
		writeError(w, http.StatusServiceUnavailable, "db unreachable")
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func migrate(ctx context.Context) error {
	_, err := db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS pets (
			id         SERIAL PRIMARY KEY,
			name       TEXT NOT NULL,
			hunger     INT NOT NULL DEFAULT 0,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			last_fed_at TIMESTAMPTZ
		)`)
	return err
}

func main() {
	ctx := context.Background()

	pool, err := pgxpool.New(ctx, "")
	if err != nil {
		log.Fatalf("db config: %v", err)
	}
	db = pool
	defer db.Close()

	if err := migrate(ctx); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /pets", createPet)
	mux.HandleFunc("GET /pets", listPets)
	mux.HandleFunc("GET /pets/{id}", getPet)
	mux.HandleFunc("POST /pets/{id}/feed", feedPet)
	mux.HandleFunc("GET /healthz", healthz)

	addr := ":8080"
	log.Printf("kubepets-api listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}
