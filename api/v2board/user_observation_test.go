package panel

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-resty/resty/v2"
)

func TestGetUserListDistinguishesNotModifiedAndEmptySuccess(t *testing.T) {
	status := http.StatusNotModified
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/server/UniProxy/user" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if status == http.StatusNotModified {
			w.WriteHeader(status)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"users":[]}`))
	}))
	defer srv.Close()
	c := &Client{client: resty.New().SetBaseURL(srv.URL)}
	unchanged, err := c.GetUserList(context.Background())
	if err != nil || unchanged != nil {
		t.Fatalf("304 must mean no change: %#v %v", unchanged, err)
	}
	status = http.StatusOK
	empty, err := c.GetUserList(context.Background())
	if err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("200 empty users must revoke all: %#v %v", empty, err)
	}
}
