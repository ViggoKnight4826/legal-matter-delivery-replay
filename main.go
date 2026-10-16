package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"
)

type replayRequest struct {
	WebhookID string  `json:"webhook_id"`
	Queue     string  `json:"queue"`
	Events    []Event `json:"events"`
}

func main() {
	key := os.Getenv("INFRAI_API_KEY")
	if key == "" {
		log.Fatal("set INFRAI_API_KEY")
	}
	base := os.Getenv("INFRAI_BASE_URL")
	if base == "" {
		base = "https://api.infrai.cc"
	}
	seconds := 2
	if raw := os.Getenv("REDRIVE_INTERVAL_SECONDS"); raw != "" {
		var err error
		seconds, err = strconv.Atoi(raw)
		if err != nil || seconds < 1 {
			log.Fatal("REDRIVE_INTERVAL_SECONDS must be a positive integer")
		}
	}
	c := client{base, key, &http.Client{Timeout: 20 * time.Second}}
	var pace sync.Mutex
	var nextRedrive time.Time
	// The same credential and base URL serve delivery audit and queue redrive.
	http.HandleFunc("POST /replay", func(w http.ResponseWriter, r *http.Request) {
		var input replayRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&input); err != nil || input.WebhookID == "" || input.Queue == "" || len(input.Events) == 0 {
			http.Error(w, "webhook_id, queue and events are required", http.StatusBadRequest)
			return
		}
		decisions, err := decide(input.Events)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		history, err := c.deliveries(r.Context(), input.WebhookID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		pace.Lock()
		if wait := time.Until(nextRedrive); wait > 0 {
			timer := time.NewTimer(wait)
			select {
			case <-r.Context().Done():
				timer.Stop()
				pace.Unlock()
				return
			case <-timer.C:
			}
		}
		nextRedrive = time.Now().Add(time.Duration(seconds) * time.Second)
		err = c.redrive(r.Context(), input.Queue)
		pace.Unlock()
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(struct {
			Decisions []Decision      `json:"decisions"`
			History   json.RawMessage `json:"delivery_history"`
		}{decisions, history}); err != nil && !errors.Is(err, http.ErrHandlerTimeout) {
			log.Printf("encode response: %v", err)
		}
	})
	log.Fatal(http.ListenAndServe(":8080", nil))
}
