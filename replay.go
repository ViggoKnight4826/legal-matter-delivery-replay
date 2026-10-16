package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Event struct {
	ID     string `json:"id"`
	Matter string `json:"matter"`
	Kind   string `json:"kind"`
}

type Decision struct {
	ID     string `json:"id"`
	Matter string `json:"matter"`
	Action string `json:"action"`
}

func decide(events []Event) ([]Decision, error) {
	seen := make(map[string]bool)
	var out []Decision
	for _, e := range events {
		if e.ID == "" || e.Matter == "" {
			return nil, errors.New("event id and matter are required")
		}
		if seen[e.ID] {
			continue
		}
		seen[e.ID] = true
		action := ""
		switch e.Kind {
		case "matter.intake":
			action = "open_matter"
		case "document.signed":
			action = "deliver_signed_document"
		case "deadline.due":
			action = "schedule_follow_up"
		default:
			return nil, fmt.Errorf("unknown event kind %q", e.Kind)
		}
		out = append(out, Decision{e.ID, e.Matter, action})
	}
	return out, nil
}

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type envelope struct {
	OK    bool            `json:"ok"`
	Data  json.RawMessage `json:"data"`
	Error apiError        `json:"error"`
}

type client struct {
	base string
	key  string
	http *http.Client
}

func (c client) call(ctx context.Context, method, path string) (json.RawMessage, error) {
	for attempt := 0; attempt < 4; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, c.base+path, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+c.key)
		res, err := c.http.Do(req)
		if err != nil {
			return nil, err
		}
		var env envelope
		decodeErr := json.NewDecoder(io.LimitReader(res.Body, 4<<20)).Decode(&env)
		res.Body.Close()
		if res.StatusCode == http.StatusTooManyRequests && attempt < 3 {
			wait := time.Duration(1<<attempt) * time.Second
			if seconds, err := strconv.Atoi(res.Header.Get("Retry-After")); err == nil && seconds >= 0 {
				wait = time.Duration(seconds) * time.Second
			}
			timer := time.NewTimer(wait)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, ctx.Err()
			case <-timer.C:
			}
			continue
		}
		if decodeErr != nil {
			return nil, fmt.Errorf("decode response: %w", decodeErr)
		}
		if !env.OK {
			return nil, fmt.Errorf("infrai response %d: %s: %s", res.StatusCode, env.Error.Code, env.Error.Message)
		}
		if res.StatusCode >= 500 {
			return nil, fmt.Errorf("upstream status %d", res.StatusCode)
		}
		return env.Data, nil
	}
	return nil, errors.New("retry limit reached")
}

func segment(s string) (string, error) {
	if s == "" || strings.ContainsAny(s, "/?#") {
		return "", errors.New("identifier must be one path segment")
	}
	return url.PathEscape(s), nil
}

func (c client) deliveries(ctx context.Context, id string) (json.RawMessage, error) {
	part, err := segment(id)
	if err != nil {
		return nil, err
	}
	return c.call(ctx, http.MethodGet, strings.Replace("/v1/account/webhooks/deliveries/{id}", "{id}", part, 1))
}

func (c client) redrive(ctx context.Context, queue string) error {
	part, err := segment(queue)
	if err != nil {
		return err
	}
	_, err = c.call(ctx, http.MethodPost, strings.Replace("/v1/queue/dlq/redrive/{queue}", "{queue}", part, 1))
	return err
}
