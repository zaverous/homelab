package main

import (
	"encoding/json"
	"log"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
)

// The chaos engine: POST /chaos/batch-feed floods the Redis hunger-queue with
// thousands of jobs in one call - the interactive counterpart to the loadgen
// CronJob, built for the platform team to trigger worker autoscaling and
// resource-limit failures on demand from the UI's Dev Mode.
//
// Jobs use the exact worker payload ({pet_id, amount}) and target ALL pets in
// the database (chaos is cluster-wide, not owner-scoped). Requires a login
// when auth is configured - this endpoint will eventually sit on the public
// internet, and an anonymous self-DoS button would be a gift to strangers.

// HungerJob mirrors worker/main.go - one queued hunger increment.
type HungerJob struct {
	PetID  int64 `json:"pet_id"`
	Amount int   `json:"amount"`
}

const chaosDefault = 5000

func chaosMax() int {
	if v, err := strconv.Atoi(envOr("CHAOS_MAX_EVENTS", "20000")); err == nil && v > 0 {
		return v
	}
	return 20000
}

func (a *app) batchFeed(w http.ResponseWriter, r *http.Request) {
	if a.auth != nil && a.sessionUser(r) == nil {
		writeError(w, http.StatusUnauthorized, "log in to unleash chaos")
		return
	}

	count := chaosDefault
	var body struct {
		Count *int `json:"count"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err == nil && body.Count != nil {
		count = *body.Count
	}
	if max := chaosMax(); count < 1 || count > max {
		writeError(w, http.StatusBadRequest, "count must be between 1 and "+strconv.Itoa(max))
		return
	}

	rows, err := a.db.Query(r.Context(), `SELECT id FROM pets`)
	if err != nil {
		log.Printf("batchFeed: list pets: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to list pets")
		return
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to read pets")
			return
		}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		writeError(w, http.StatusConflict, "no pets exist to torment")
		return
	}

	// Chunked LPUSH, same shape as the loadgen: ~1000 payloads per round trip.
	// Redis noeviction OOM mid-flood is not an error - it's the designed
	// backpressure (ADR-005); report the partial count honestly.
	const chunk = 1000
	enqueued := 0
	backpressure := false
	buf := make([]any, 0, chunk)
	flush := func() bool {
		if len(buf) == 0 {
			return true
		}
		if err := a.rdb.LPush(r.Context(), a.queueKey, buf...).Err(); err != nil {
			if strings.Contains(err.Error(), "OOM") {
				backpressure = true
				return false
			}
			log.Printf("batchFeed: lpush: %v", err)
			writeError(w, http.StatusBadGateway, "queue unreachable after "+strconv.Itoa(enqueued)+" events")
			return false
		}
		enqueued += len(buf)
		buf = buf[:0]
		return true
	}

	for i := 0; i < count; i++ {
		payload, err := json.Marshal(HungerJob{
			PetID:  ids[rand.Intn(len(ids))],
			Amount: rand.Intn(10) + 1,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "marshal failure")
			return
		}
		buf = append(buf, payload)
		if len(buf) >= chunk {
			if !flush() {
				if !backpressure {
					return // hard redis error - response already written
				}
				break
			}
		}
	}
	if !backpressure {
		if !flush() && !backpressure {
			return
		}
	}

	depth, _ := a.rdb.LLen(r.Context(), a.queueKey).Result()
	writeJSON(w, http.StatusOK, map[string]any{
		"requested":    count,
		"enqueued":     enqueued,
		"queue_depth":  depth,
		"backpressure": backpressure,
	})
}
