package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestDecide(t *testing.T) {
	tests := []struct {
		name   string
		events []Event
		want   []Decision
		bad    bool
	}{
		{"intake and duplicate", []Event{{"a", "M1", "matter.intake"}, {"a", "M1", "matter.intake"}}, []Decision{{"a", "M1", "open_matter"}}, false},
		{"signed and due", []Event{{"b", "M2", "document.signed"}, {"c", "M2", "deadline.due"}}, []Decision{{"b", "M2", "deliver_signed_document"}, {"c", "M2", "schedule_follow_up"}}, false},
		{"unknown kind", []Event{{"d", "M3", "other"}}, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := decide(tt.events)
			if (err != nil) != tt.bad || !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("decide() = %v, %v; want %v, bad=%v", got, err, tt.want, tt.bad)
			}
		})
	}
}

func TestEnvelopeBeforeStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/account/webhooks/deliveries/hook1" || r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"ok":false,"error":{"code":"INVALID_ARGUMENT","message":"bad id"}}`))
	}))
	defer srv.Close()
	c := client{srv.URL, "test-key", srv.Client()}
	_, err := c.deliveries(context.Background(), "hook1")
	if err == nil || err.Error() != "infrai response 400: INVALID_ARGUMENT: bad id" {
		t.Fatalf("envelope error = %v", err)
	}
}
