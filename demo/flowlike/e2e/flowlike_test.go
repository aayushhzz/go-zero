package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// CodeBuild starts live dependencies and both processes before enabling this test.
func TestFlowlikeE2E(t *testing.T) {
	if os.Getenv("FLOWLIKE_E2E") != "1" {
		t.Skip("requires the isolated Flow-like CodeBuild stack")
	}
	client := &http.Client{Timeout: 5 * time.Second}
	request, err := http.NewRequest(http.MethodPost, "http://127.0.0.1:8099/jobs", bytes.NewBufferString(`{"name":"flow-demo"}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("POST status %d: %s", response.StatusCode, body)
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(response.Body).Decode(&created); err != nil || created.ID == "" {
		t.Fatalf("invalid create response: id=%q err=%v", created.ID, err)
	}
	redisClient := redis.NewClient(&redis.Options{Addr: "127.0.0.1:6379"})
	defer redisClient.Close()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		get, err := client.Get("http://127.0.0.1:8099/jobs/" + created.ID)
		if err != nil {
			t.Fatal(err)
		}
		var job struct {
			Status string `json:"status"`
		}
		err = json.NewDecoder(get.Body).Decode(&job)
		get.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		value, err := redisClient.Get(context.Background(), "flowlike:done:"+created.ID).Result()
		if job.Status == "done" && err == nil && value == "1" {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatal("worker did not update MongoDB and Redis before deadline")
}
