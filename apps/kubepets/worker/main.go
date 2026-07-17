package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type HungerJob struct {
	PetID  int64 `json:"pet_id"`
	Amount int   `json:"amount"`
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	ctx := context.Background()

	redisAddr := envOr("REDIS_ADDR", "redis.kubepets.svc.cluster.local:6379")
	queueKey := envOr("QUEUE_KEY", "hunger-queue")
	batchSize, err := strconv.Atoi(envOr("BATCH_SIZE", "10"))
	if err != nil || batchSize < 1 {
		log.Fatalf("invalid BATCH_SIZE: %v", envOr("BATCH_SIZE", "10"))
	}

	rdb := redis.NewClient(&redis.Options{Addr: redisAddr})
	defer rdb.Close()
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatalf("redis connect: %v", err)
	}

	db, err := pgxpool.New(ctx, "")
	if err != nil {
		log.Fatalf("db config: %v", err)
	}
	defer db.Close()

	log.Printf("kubepets-worker started (queue=%s, batch_size=%d)", queueKey, batchSize)

	for {
		// Blocking pop for the first job in the batch - avoids busy-looping
		// when the queue is empty.
		res, err := rdb.BRPop(ctx, 5*time.Second, queueKey).Result()
		if errors.Is(err, redis.Nil) {
			continue // timed out, no job - loop and block again
		}
		if err != nil {
			log.Printf("brpop error: %v", err)
			time.Sleep(time.Second)
			continue
		}

		batch := make([]HungerJob, 0, batchSize)
		if job, ok := decodeJob(res[1]); ok {
			batch = append(batch, job)
		}

		// Drain up to batchSize-1 more jobs WITHOUT blocking, holding them all
		// in memory before writing anything - this is deliberate: a large
		// BATCH_SIZE means a large in-memory batch, which is the knob used
		// later to push this worker past its own memory limit on demand.
		for len(batch) < batchSize {
			val, err := rdb.RPop(ctx, queueKey).Result()
			if errors.Is(err, redis.Nil) {
				break // queue drained for now
			}
			if err != nil {
				log.Printf("rpop error: %v", err)
				break
			}
			if job, ok := decodeJob(val); ok {
				batch = append(batch, job)
			}
		}

		applyBatch(ctx, db, batch)
	}
}

func decodeJob(raw string) (HungerJob, bool) {
	var job HungerJob
	if err := json.Unmarshal([]byte(raw), &job); err != nil {
		log.Printf("bad job payload, skipping: %v", err)
		return job, false
	}
	return job, true
}

func applyBatch(ctx context.Context, db *pgxpool.Pool, batch []HungerJob) {
	applied := 0
	for _, job := range batch {
		tag, err := db.Exec(ctx,
			`UPDATE pets SET hunger = LEAST(hunger + $1, 100) WHERE id = $2`,
			job.Amount, job.PetID)
		if err != nil {
			log.Printf("update failed for pet %d: %v", job.PetID, err)
			continue
		}
		if tag.RowsAffected() > 0 {
			applied++
		}
	}
	log.Printf("batch: %d/%d jobs applied", applied, len(batch))
}
