package panel

import (
	"context"
	"fmt"
)

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
	const path = "/api/v1/server/UniProxy/observation"
	r, err := c.client.R().SetContext(ctx).SetHeader("X-Observation-Token", c.ObservationToken).SetBody(map[string]any{"events": events}).ForceContentType("application/json").Post(path)
	if err != nil {
		return err
	}
	if r == nil {
		return fmt.Errorf("report observation failed: received nil response")
	}
	if r.IsError() {
		return fmt.Errorf("report observation failed: status %d", r.StatusCode())
	}
	return nil
}
