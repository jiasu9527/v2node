package panel

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-resty/resty/v2"
)

func TestReportNodeOnlineUsersReturnsHTTPError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/server/UniProxy/alive" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	client := &Client{
		client: resty.New().SetBaseURL(srv.URL),
	}

	data := map[int][]string{1: {"203.0.113.10"}}
	err := client.ReportNodeOnlineUsers(context.Background(), &data)
	if err == nil {
		t.Fatal("ReportNodeOnlineUsers() error = nil, want non-nil")
	}
}

func TestReportUserTrafficReturnsHTTPError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/server/UniProxy/push" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	client := &Client{
		client: resty.New().SetBaseURL(srv.URL),
	}

	data := []UserTraffic{{UID: 1, Upload: 1024, Download: 2048}}
	err := client.ReportUserTraffic(context.Background(), data)
	if err == nil {
		t.Fatal("ReportUserTraffic() error = nil, want non-nil")
	}
}

func TestReportNodeOnlineUsersSendsBody(t *testing.T) {
	t.Parallel()

	gotMethod := ""
	gotContentType := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotContentType = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := &Client{
		client: resty.New().SetBaseURL(srv.URL),
	}

	data := map[int][]string{1: {"203.0.113.10", "203.0.113.11"}}
	if err := client.ReportNodeOnlineUsers(context.Background(), &data); err != nil {
		t.Fatalf("ReportNodeOnlineUsers() error = %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Fatalf("request method = %q, want %q", gotMethod, http.MethodPost)
	}
	if gotContentType == "" {
		t.Fatal("content type header was not set")
	}
}

func TestReportNodeOnlineUsersReturnsTransportError(t *testing.T) {
	t.Parallel()

	client := &Client{
		client: resty.New().SetBaseURL("http://127.0.0.1:1"),
	}

	data := map[int][]string{1: {"203.0.113.10"}}
	err := client.ReportNodeOnlineUsers(context.Background(), &data)
	if err == nil {
		t.Fatal("ReportNodeOnlineUsers() error = nil, want non-nil")
	}
	var opErr interface{ Unwrap() error }
	if !errors.As(err, &opErr) {
		t.Fatalf("ReportNodeOnlineUsers() error = %T, want wrapped transport error", err)
	}
}
