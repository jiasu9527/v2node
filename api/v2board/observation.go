package panel

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/go-resty/resty/v2"
)

var ErrObservationInvalidBatch = errors.New("observation batch rejected as invalid")

// ObservationEvent reports destination metadata only. Result=route_attempt
// means the proxy accepted a routed request, not that a website was reached.
type ObservationEvent struct {
	EventID       string `json:"event_id"`
	UserID        int    `json:"user_id"`
	TargetHost    string `json:"target_host,omitempty"`
	TargetIP      string `json:"target_ip,omitempty"`
	TargetPort    int    `json:"target_port"`
	Network       string `json:"network"`
	ClientIP      string `json:"client_ip,omitempty"`
	OccurredAt    int64  `json:"occurred_at"`
	Result        string `json:"result"`
	UplinkBytes   int64  `json:"uplink_bytes"`
	DownlinkBytes int64  `json:"downlink_bytes"`
}

func (c *Client) ReportObservation(ctx context.Context, events []ObservationEvent) error {
	if len(events) == 0 {
		return nil
	}
	// The ordinary panel client carries a shared server token in its default
	// query parameters. Use an isolated client so that token is never sent to
	// this endpoint (or captured in URL access logs).
	client := resty.New().SetBaseURL(c.APIHost).SetTimeout(15 * time.Second).SetRetryCount(1)
	const path = "/api/v1/server/UniProxy/observation"
	r, err := client.R().SetContext(ctx).
		SetQueryParams(map[string]string{"node_type": "v2node", "node_id": strconv.Itoa(c.NodeId)}).
		SetHeader("X-Observation-Token", c.ObservationToken).
		SetBody(map[string]any{"events": events}).
		ForceContentType("application/json").Post(path)
	if err != nil {
		return err
	}
	if r == nil {
		return fmt.Errorf("report observation failed: received nil response")
	}
	if r.IsError() {
		if r.StatusCode() == 400 {
			return fmt.Errorf("%w: status %d", ErrObservationInvalidBatch, r.StatusCode())
		}
		return fmt.Errorf("report observation failed: status %d", r.StatusCode())
	}
	return nil
}
