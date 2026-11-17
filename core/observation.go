package core

import (
	"strings"

	panel "github.com/wyx2685/v2node/api/v2board"
)

func (v *V2Core) configureObservationAudit(tag string, info *panel.NodeInfo) {
	if v == nil || v.dispatcher == nil || info == nil || info.Common == nil {
		return
	}
	audit := info.Common.ObservationAudit
	v.dispatcher.ConfigureObservationAudit(tag, audit != nil && audit.Enable)
}

func (v *V2Core) disableObservationAudit(tag string) {
	if v == nil || v.dispatcher == nil {
		return
	}
	v.dispatcher.ConfigureObservationAudit(tag, false)
}

// GetObservationSlice maps the authenticated Xray identity back to the panel
// user ID. It never includes the user's UUID in the wire event.
func (v *V2Core) GetObservationSlice(tag string) ([]panel.ObservationEvent, uint64) {
	if v == nil || v.dispatcher == nil || v.users == nil {
		return nil, 0
	}
	uidByEmail := v.users.Snapshot()
	raw, dropped := v.dispatcher.DrainObservation(tag)
	result := make([]panel.ObservationEvent, 0, len(raw))
	for _, event := range raw {
		uid := uidByEmail[event.Email]
		if uid <= 0 || (strings.TrimSpace(event.Domain) == "" && strings.TrimSpace(event.TargetIP) == "") {
			continue
		}
		result = append(result, panel.ObservationEvent{
			EventID: event.EventID, UserID: uid,
			TargetHost: event.Domain, TargetIP: event.TargetIP,
			TargetPort: event.TargetPort, Network: event.Network,
			ClientIP: event.ClientIP, OccurredAt: event.StartedAt,
			Result: event.Result,
		})
	}
	return result, dropped
}
