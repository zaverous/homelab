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

const (
	hungerTickSeconds = 120
	feedRelief        = 25
)

// advanceHunger materializes elapsed time in whole ticks while preserving the
// remainder. Polling frequently therefore cannot postpone the next tick.
func (a *app) advanceHunger(r *http.Request, ownerID int64) error {
	_, err := a.db.Exec(r.Context(),
		`UPDATE pets
		 SET hunger = LEAST(100, hunger + FLOOR(EXTRACT(EPOCH FROM (now() - hunger_updated_at)) / $1::double precision)::int),
		     hunger_updated_at = hunger_updated_at
		       + make_interval(secs => (
		           FLOOR(EXTRACT(EPOCH FROM (now() - hunger_updated_at)) / $1::double precision) * $1
		         )::double precision)
		 WHERE owner_id = $2
		   AND hunger < 100
		   AND now() - hunger_updated_at >= make_interval(secs => $1::double precision)`,
		hungerTickSeconds, ownerID)
	return err
}

func (a *app) createPet(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
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
		body.Name, u.ID)
	pet, err := scanPet(row)
	if err != nil {
		log.Printf("createPet: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to create pet")
		return
	}
	writeJSON(w, http.StatusCreated, pet)
}

func (a *app) listPets(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	if err := a.advanceHunger(r, u.ID); err != nil {
		log.Printf("listPets advance hunger: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to update hunger")
		return
	}
	rows, err := a.db.Query(r.Context(),
		`SELECT id, name, hunger, created_at, last_fed_at FROM pets
		 WHERE owner_id = $1 ORDER BY id`, u.ID)
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
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	if err := a.advanceHunger(r, u.ID); err != nil {
		log.Printf("getPet advance hunger: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to update hunger")
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid pet id")
		return
	}
	row := a.db.QueryRow(r.Context(),
		`SELECT id, name, hunger, created_at, last_fed_at FROM pets
		 WHERE id = $1 AND owner_id = $2`, id, u.ID)
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
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid pet id")
		return
	}
	if err := a.advanceHunger(r, u.ID); err != nil {
		log.Printf("feedPet advance hunger: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to update hunger")
		return
	}
	row := a.db.QueryRow(r.Context(),
		`UPDATE pets
		 SET hunger = GREATEST(0, hunger - $1),
		     hunger_updated_at = now(),
		     last_fed_at = now()
		 WHERE id = $2 AND owner_id = $3
		 RETURNING id, name, hunger, created_at, last_fed_at`,
		feedRelief, id, u.ID)
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
