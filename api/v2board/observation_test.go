package panel

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-resty/resty/v2"
)

func TestReportObservationUsesDedicatedTokenAndStablePayload(t *testing.T) {
	token := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	var got ObservationEvent
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/server/UniProxy/observation" || r.Header.Get("X-Observation-Token") != token {
			t.Errorf("wrong endpoint or token header: %q %q", r.URL.Path, r.Header.Get("X-Observation-Token"))
		}
		var body struct {
			Events []ObservationEvent `json:"events"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.Events) != 1 {
			t.Errorf("decode payload: %v %#v", err, body)
			return
		}
		got = body.Events[0]
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"inserted":1}}`))
	}))
	defer srv.Close()
	c := &Client{client: resty.New().SetBaseURL(srv.URL), ObservationToken: token}
	event := ObservationEvent{EventID: "abcdef0123456789", UserID: 123, TargetHost: "example.com", TargetPort: 443, Network: "tcp", OccurredAt: 1700000000, Result: "route_attempt"}
	if err := c.ReportObservation(context.Background(), []ObservationEvent{event}); err != nil {
		t.Fatal(err)
	}
	if got != event {
		t.Fatalf("got %#v, want %#v", got, event)
	}
}
