package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
)

type Pet struct {
	ID        int64      `json:"id"`
	Name      string     `json:"name"`
	Hunger    int        `json:"hunger"`
	CreatedAt time.Time  `json:"created_at"`
	LastFedAt *time.Time `json:"last_fed_at,omitempty"`
}

// ownerScope maps the caller to the owner_id used in queries: a logged-in
// user sees exactly their pets; anonymous callers see the strays (owner_id
// NULL) - which keeps the pre-auth UI and the in-cluster loadgen (which calls
// the API unauthenticated) working unchanged. Queries compare with
// IS NOT DISTINCT FROM so the nil pointer genuinely matches NULL rows.
func ownerScope(u *SessionUser) *int64 {
	if u == nil {
		return nil
	}
	return &u.ID
}

func (a *app) createPet(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	row := a.db.QueryRow(r.Context(),
		`INSERT INTO pets (name, owner_id) VALUES ($1, $2)
		 RETURNING id, name, hunger, created_at, last_fed_at`,
		body.Name, ownerScope(a.sessionUser(r)))
	pet, err := scanPet(row)
	if err != nil {
		log.Printf("createPet: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to create pet")
		return
	}
	writeJSON(w, http.StatusCreated, pet)
}

func (a *app) listPets(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.Query(r.Context(),
		`SELECT id, name, hunger, created_at, last_fed_at FROM pets
		 WHERE owner_id IS NOT DISTINCT FROM $1 ORDER BY id`,
		ownerScope(a.sessionUser(r)))
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

func (a *app) getPet(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid pet id")
		return
	}
	row := a.db.QueryRow(r.Context(),
		`SELECT id, name, hunger, created_at, last_fed_at FROM pets
		 WHERE id = $1 AND owner_id IS NOT DISTINCT FROM $2`,
		id, ownerScope(a.sessionUser(r)))
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

func (a *app) feedPet(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid pet id")
		return
	}
	row := a.db.QueryRow(r.Context(),
		`UPDATE pets SET hunger = 0, last_fed_at = now()
		 WHERE id = $1 AND owner_id IS NOT DISTINCT FROM $2
		 RETURNING id, name, hunger, created_at, last_fed_at`,
		id, ownerScope(a.sessionUser(r)))
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
