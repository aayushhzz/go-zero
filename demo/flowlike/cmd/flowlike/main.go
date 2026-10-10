package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/zeromicro/go-zero/demo/flowlike"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func main() {
	role := flag.String("role", "api", "api or worker")
	flag.Parse()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	mongoURI := env("FLOWLIKE_MONGO_URI", "mongodb://127.0.0.1:27017/?directConnection=true&replicaSet=rs0")
	redisAddr := env("FLOWLIKE_REDIS_ADDR", "127.0.0.1:6379")
	mc, err := mongo.Connect(options.Client().ApplyURI(mongoURI))
	if err != nil {
		log.Fatal(err)
	}
	defer mc.Disconnect(context.Background())
	rc := redis.NewClient(&redis.Options{Addr: redisAddr})
	defer rc.Close()
	svc := &flowlike.Service{Jobs: mc.Database("flowlike_e2e").Collection("jobs"), Redis: rc}
	switch *role {
	case "worker":
		if err := svc.RunWorker(ctx); err != nil && !errors.Is(err, context.Canceled) {
			log.Fatal(err)
		}
	case "api":
		server := &http.Server{Addr: "127.0.0.1:8099", Handler: svc.Handler(), ReadHeaderTimeout: 5 * time.Second}
		go func() {
			<-ctx.Done()
			shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = server.Shutdown(shutdown)
		}()
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	default:
		log.Fatalf("unknown role: %s", *role)
	}
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
