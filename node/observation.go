package node

import (
	"context"
	"errors"
	"fmt"
	"strings"

	log "github.com/sirupsen/logrus"
	panel "github.com/wyx2685/v2node/api/v2board"
)

const maxPendingObservationEvents = 8192
const maxObservationReportBatch = 100

func observationEnabled(info *panel.NodeInfo) bool {
	return info != nil && info.Common != nil && info.Common.ObservationAudit != nil && info.Common.ObservationAudit.Enable
}

// filterObservationUsers is a second, node-side authentication boundary for
// a dedicated observation node. Subscription policy alone is not sufficient.
func filterObservationUsers(info *panel.NodeInfo, users []panel.UserInfo) ([]panel.UserInfo, error) {
	if !observationEnabled(info) {
		return users, nil
	}
	if isExternalNode(info) {
		return nil, errors.New("observation requires an Xray-backed node")
	}
	ids := info.Common.ObservationAudit.AllowedUserIDs
	if len(ids) == 0 {
		return nil, errors.New("observation allowlist is empty")
	}
	allowed := make(map[int]struct{}, len(ids))
	for _, id := range ids {
		if id <= 0 {
			return nil, fmt.Errorf("invalid observation user ID %d", id)
		}
		allowed[id] = struct{}{}
	}
	filtered := make([]panel.UserInfo, 0, len(users))
	for _, user := range users {
		if _, ok := allowed[user.Id]; ok {
			filtered = append(filtered, user)
		}
	}
	if len(filtered) == 0 {
		return nil, errors.New("observation allowlist has no active users")
	}
	return filtered, nil
}

func (c *Controller) reportObservationTask(ctx context.Context) error {
	if !observationEnabled(c.info) || strings.TrimSpace(c.apiClient.ObservationToken) == "" {
		return nil
	}
	fresh, dropped := c.server.GetObservationSlice(c.tag)
	if dropped > 0 {
		log.WithFields(log.Fields{"tag": c.tag, "dropped": dropped}).Warn("Observation buffer overflow")
	}
	if dropped := c.appendPendingObservation(fresh); dropped > 0 {
		log.WithFields(log.Fields{"tag": c.tag, "dropped": dropped}).Warn("Observation retry queue overflow")
	}
	if err := c.flushPendingObservation(ctx, c.apiClient.ReportObservation); err != nil {
		log.WithFields(log.Fields{"tag": c.tag, "err": err, "pending": len(c.pendingObservation)}).Warn("Report observation failed; events queued for retry")
		// Task.Start stops the periodic task on a non-context error. Keep it
		// running so the stable-ID batch can be retried on the next tick.
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		return nil
	}
	return nil
}

func (c *Controller) appendPendingObservation(events []panel.ObservationEvent) int {
	c.pendingObservation = append(c.pendingObservation, events...)
	if len(c.pendingObservation) <= maxPendingObservationEvents {
		return 0
	}
	dropped := len(c.pendingObservation) - maxPendingObservationEvents
	c.pendingObservation = append([]panel.ObservationEvent(nil), c.pendingObservation[dropped:]...)
	return dropped
}

func (c *Controller) flushPendingObservation(ctx context.Context, report func(context.Context, []panel.ObservationEvent) error) error {
	for len(c.pendingObservation) > 0 {
		batchSize := len(c.pendingObservation)
		if batchSize > maxObservationReportBatch {
			batchSize = maxObservationReportBatch
		}
		if err := report(ctx, c.pendingObservation[:batchSize]); err != nil {
			// Keep the same event IDs and retry later. The panel deduplicates IDs.
			return err
		}
		c.pendingObservation = c.pendingObservation[batchSize:]
	}
	return nil
}
