package flowlike

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

const queue = "flowlike:jobs"

// Service models an API and a separate worker sharing MongoDB and Redis.
type Service struct {
	Jobs  *mongo.Collection
	Redis *redis.Client
}

func (s *Service) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		if err := s.Jobs.Database().Client().Ping(ctx, nil); err != nil {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}
		if err := s.Redis.Ping(ctx).Err(); err != nil {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /jobs", func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil || strings.TrimSpace(request.Name) == "" {
			http.Error(w, "name is required", http.StatusBadRequest)
			return
		}
		id := uuid.NewString()
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		if _, err := s.Jobs.InsertOne(ctx, bson.M{"_id": id, "name": request.Name, "status": "queued"}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := s.Redis.LPush(ctx, queue, id).Err(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(map[string]string{"id": id})
	})
	mux.HandleFunc("GET /jobs/{id}", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		var job struct {
			ID     string `bson:"_id" json:"id"`
			Name   string `bson:"name" json:"name"`
			Status string `bson:"status" json:"status"`
		}
		if err := s.Jobs.FindOne(ctx, bson.M{"_id": r.PathValue("id")}).Decode(&job); err != nil {
			if errors.Is(err, mongo.ErrNoDocuments) {
				http.NotFound(w, r)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(job)
	})
	return mux
}

func (s *Service) RunWorker(ctx context.Context) error {
	for ctx.Err() == nil {
		result, err := s.Redis.BRPop(ctx, time.Second, queue).Result()
		if errors.Is(err, redis.Nil) {
			continue
		}
		if err != nil {
			return err
		}
		id := result[1]
		work, cancel := context.WithTimeout(ctx, 5*time.Second)
		_, err = s.Jobs.UpdateOne(work, bson.M{"_id": id}, bson.M{"$set": bson.M{"status": "done"}})
		if err == nil {
			err = s.Redis.Set(work, "flowlike:done:"+id, "1", time.Minute).Err()
		}
		cancel()
		if err != nil {
			return err
		}
	}
	return ctx.Err()
}
