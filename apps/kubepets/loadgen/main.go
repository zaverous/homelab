package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// HungerJob is the exact payload the worker decodes (see worker/main.go).
type HungerJob struct {
	PetID  int64 `json:"pet_id"`
	Amount int   `json:"amount"`
}

// Pet is the subset of the API's pet shape we need to pick a target.
type Pet struct {
	ID int64 `json:"id"`
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			log.Fatalf("invalid %s=%q: %v", key, v, err)
		}
		return n
	}
	return def
}

func main() {
	ctx := context.Background()

	redisAddr := envOr("REDIS_ADDR", "redis.kubepets.svc.cluster.local:6379")
	queueKey := envOr("QUEUE_KEY", "hunger-queue")
	apiAddr := strings.TrimRight(envOr("API_ADDR", "http://kubepets-api.kubepets.svc.cluster.local:80"), "/")
	events := envInt("EVENTS_PER_RUN", 10000)
	maxAmount := envInt("MAX_AMOUNT", 10)
	seedPets := envInt("SEED_PETS", 5)
	maxQueue := envInt("MAX_QUEUE", 0) // 0 = disabled (let it run away for the OOM demo)

	if events < 1 || maxAmount < 1 {
		log.Fatalf("EVENTS_PER_RUN and MAX_AMOUNT must be >= 1")
	}

	rdb := redis.NewClient(&redis.Options{Addr: redisAddr})
	defer rdb.Close()
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatalf("redis connect: %v", err)
	}

	// Safety valve: if the queue is already deeper than MAX_QUEUE, skip this
	// run entirely. Default 0 disables it - during the chaos endgame you WANT
	// the backlog to grow unbounded and drive the worker OOM; Redis's own
	// noeviction cap (ADR-005) is the real backpressure. Set it >0 for gentle,
	// steady-state load that won't fill Redis.
	if maxQueue > 0 {
		depth, err := rdb.LLen(ctx, queueKey).Result()
		if err != nil {
			log.Fatalf("llen: %v", err)
		}
		if depth >= int64(maxQueue) {
			log.Printf("queue depth %d >= MAX_QUEUE %d - skipping this run", depth, maxQueue)
			return
		}
	}

	// Discover real pet IDs so every enqueued job actually lands - the worker
	// silently skips unknown pet_ids (RowsAffected 0). Self-seed a handful if
	// the DB is empty so a fresh cluster still produces a visible demo.
	ids := ensurePets(apiAddr, seedPets)
	if len(ids) == 0 {
		log.Fatalf("no pet IDs available and seeding failed - cannot enqueue")
	}
	log.Printf("targeting %d pets, enqueuing %d events into %q", len(ids), events, queueKey)

	// Marshal all payloads, then LPUSH in chunks. One LPUSH per job would be
	// 10k round trips/min; chunking cuts that ~1000x. LPUSH (left) pairs with
	// the worker's BRPOP/RPOP (right) for FIFO ordering.
	const chunk = 1000
	buf := make([]interface{}, 0, chunk)
	pushed := 0
	flush := func() (bool, error) {
		if len(buf) == 0 {
			return true, nil
		}
		if err := rdb.LPush(ctx, queueKey, buf...).Err(); err != nil {
			// Redis returns "OOM command not allowed..." once it hits maxmemory
			// under noeviction. That's not a failure - it's the designed
			// backpressure kicking in. Log and stop cleanly (exit 0) so the
			// CronJob doesn't spin on backoff retries.
			if strings.Contains(err.Error(), "OOM") {
				return false, nil
			}
			return false, err
		}
		pushed += len(buf)
		buf = buf[:0]
		return true, nil
	}

	for i := 0; i < events; i++ {
		job := HungerJob{
			PetID:  ids[rand.Intn(len(ids))],
			Amount: rand.Intn(maxAmount) + 1,
		}
		payload, err := json.Marshal(job)
		if err != nil {
			log.Fatalf("marshal job: %v", err)
		}
		buf = append(buf, payload)
		if len(buf) >= chunk {
			ok, err := flush()
			if err != nil {
				log.Fatalf("lpush after %d events: %v", pushed, err)
			}
			if !ok {
				log.Printf("redis OOM after %d events - queue full, backpressure engaged (expected in the chaos demo)", pushed)
				return
			}
		}
	}
	if ok, err := flush(); err != nil {
		log.Fatalf("lpush after %d events: %v", pushed, err)
	} else if !ok {
		log.Printf("redis OOM after %d events - queue full, backpressure engaged (expected in the chaos demo)", pushed)
		return
	}

	depth, _ := rdb.LLen(ctx, queueKey).Result()
	log.Printf("done: enqueued %d events, queue depth now %d", pushed, depth)
}

var httpClient = &http.Client{Timeout: 10 * time.Second}

// ensurePets returns existing pet IDs, creating up to `want` of them if the DB
// is empty so the generator is self-bootstrapping on a fresh cluster.
func ensurePets(apiAddr string, want int) []int64 {
	ids, err := listPetIDs(apiAddr)
	if err != nil {
		log.Printf("list pets failed (%v) - will try to seed", err)
	}
	for len(ids) < want {
		id, err := createPet(apiAddr, fmt.Sprintf("loadgen-%d", len(ids)+1))
		if err != nil {
			log.Printf("seed pet failed: %v", err)
			break
		}
		ids = append(ids, id)
	}
	return ids
}

func listPetIDs(apiAddr string) ([]int64, error) {
	resp, err := httpClient.Get(apiAddr + "/pets")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET /pets: status %d", resp.StatusCode)
	}
	var pets []Pet
	if err := json.NewDecoder(resp.Body).Decode(&pets); err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(pets))
	for _, p := range pets {
		ids = append(ids, p.ID)
	}
	return ids, nil
}

func createPet(apiAddr, name string) (int64, error) {
	body, _ := json.Marshal(map[string]string{"name": name})
	resp, err := httpClient.Post(apiAddr+"/pets", "application/json", bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		msg, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("POST /pets: status %d: %s", resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	var pet Pet
	if err := json.NewDecoder(resp.Body).Decode(&pet); err != nil {
		return 0, err
	}
	return pet.ID, nil
}
