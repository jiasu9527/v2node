package core

import (
	"strings"

	panel "github.com/wyx2685/v2node/api/v2board"
)

func (v *V2Core) configureSensitiveAudit(tag string, info *panel.NodeInfo) {
	if v == nil || v.dispatcher == nil || info == nil || info.Common == nil || info.Common.SensitiveAudit == nil {
		return
	}
	audit := info.Common.SensitiveAudit
	v.dispatcher.ConfigureSensitiveAudit(tag, audit.Enable, audit.Rules, audit.LogClientIP)
}

func (v *V2Core) disableSensitiveAudit(tag string) {
	if v == nil || v.dispatcher == nil {
		return
	}
	v.dispatcher.ConfigureSensitiveAudit(tag, false, nil, false)
}

func (v *V2Core) GetSensitiveAccessSlice(tag string) []panel.SensitiveAccessEvent {
	if v == nil || v.dispatcher == nil {
		return nil
	}
	uidSnapshot := v.users.Snapshot()
	rawEvents := v.dispatcher.DrainSensitiveAccess(tag)
	if len(rawEvents) == 0 {
		return nil
	}
	result := make([]panel.SensitiveAccessEvent, 0, len(rawEvents))
	for _, event := range rawEvents {
		uid, ok := uidSnapshot[event.Email]
		if !ok || uid <= 0 || strings.TrimSpace(event.Domain) == "" {
			continue
		}
		result = append(result, panel.SensitiveAccessEvent{
			UserID:   uid,
			Domain:   event.Domain,
			Rule:     event.Rule,
			ClientIP: event.ClientIP,
			Count:    event.Count,
			FirstAt:  event.FirstAt,
			LastAt:   event.LastAt,
		})
	}
	if len(result) == 0 {
		return nil
	}
	return result
}
