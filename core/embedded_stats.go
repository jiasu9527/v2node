package core

import (
	"net"
	"strings"
	"sync"
	"time"

	panel "github.com/wyx2685/v2node/api/v2board"
)

type embeddedTrafficKey struct {
	NodeID int
	UUID   string
}

type embeddedTrafficRegistry struct {
	mu       sync.Mutex
	traffic  map[embeddedTrafficKey]externalTrafficSnapshot
	clientIP map[embeddedTrafficKey]string
	access   map[embeddedTrafficKey][]panel.SensitiveAccessEvent
}

var globalEmbeddedTraffic = &embeddedTrafficRegistry{traffic: make(map[embeddedTrafficKey]externalTrafficSnapshot), clientIP: make(map[embeddedTrafficKey]string), access: make(map[embeddedTrafficKey][]panel.SensitiveAccessEvent)}

type embeddedTrafficObserver struct {
	nodeID      int
	auditEnable bool
	auditRules  []embeddedSensitiveRule
	logClientIP bool
}

type embeddedSensitiveRule struct {
	Raw     string
	Kind    string
	Pattern string
}

func newEmbeddedTrafficObserver(node *panel.NodeInfo) embeddedTrafficObserver {
	observer := embeddedTrafficObserver{}
	if node == nil {
		return observer
	}
	observer.nodeID = node.Id
	if node.Common == nil || node.Common.SensitiveAudit == nil || !node.Common.SensitiveAudit.Enable {
		return observer
	}
	for _, raw := range node.Common.SensitiveAudit.Rules {
		if rule, ok := parseEmbeddedSensitiveRule(raw); ok {
			observer.auditRules = append(observer.auditRules, rule)
		}
	}
	if len(observer.auditRules) > 0 {
		observer.auditEnable = true
		observer.logClientIP = node.Common.SensitiveAudit.LogClientIP
	}
	return observer
}

func (o embeddedTrafficObserver) AddTraffic(uuid string, upload int64, download int64, clientIP string) {
	if o.nodeID <= 0 || uuid == "" || upload+download <= 0 {
		return
	}
	globalEmbeddedTraffic.add(o.nodeID, uuid, upload, download, clientIP)
}

func (o embeddedTrafficObserver) AddAccess(uuid string, domain string, network string, clientIP string) {
	if o.nodeID <= 0 || uuid == "" || strings.TrimSpace(domain) == "" {
		return
	}
	rule, ok := o.matchAccessRule(domain)
	if !ok {
		return
	}
	if !o.logClientIP {
		clientIP = ""
	}
	globalEmbeddedTraffic.addAccess(o.nodeID, uuid, domain, rule.Raw, clientIP)
}

func (o embeddedTrafficObserver) matchAccessRule(domain string) (embeddedSensitiveRule, bool) {
	if !o.auditEnable || len(o.auditRules) == 0 {
		return embeddedSensitiveRule{}, false
	}
	domain = normalizeEmbeddedSensitiveDomain(domain)
	if domain == "" {
		return embeddedSensitiveRule{}, false
	}
	for _, rule := range o.auditRules {
		switch rule.Kind {
		case "domain":
			if domain == rule.Pattern {
				return rule, true
			}
		case "suffix":
			if domain == rule.Pattern || strings.HasSuffix(domain, "."+rule.Pattern) {
				return rule, true
			}
		case "keyword":
			if strings.Contains(domain, rule.Pattern) {
				return rule, true
			}
		}
	}
	return embeddedSensitiveRule{}, false
}

func (r *embeddedTrafficRegistry) add(nodeID int, uuid string, upload int64, download int64, clientIP string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := embeddedTrafficKey{NodeID: nodeID, UUID: uuid}
	s := r.traffic[key]
	s.Upload += upload
	s.Download += download
	r.traffic[key] = s
	if clientIP != "" {
		r.clientIP[key] = clientIP
	}
}

func (r *embeddedTrafficRegistry) snapshot(nodeID int, uuidByUID map[int]string) map[int]externalTrafficSnapshot {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make(map[int]externalTrafficSnapshot)
	for uid, uuid := range uuidByUID {
		if s, ok := r.traffic[embeddedTrafficKey{NodeID: nodeID, UUID: uuid}]; ok {
			result[uid] = s
		}
	}
	return result
}

func (r *embeddedTrafficRegistry) addAccess(nodeID int, uuid string, domain string, rule string, clientIP string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := embeddedTrafficKey{NodeID: nodeID, UUID: uuid}
	now := time.Now().Unix()
	r.access[key] = append(r.access[key], panel.SensitiveAccessEvent{Domain: normalizeEmbeddedSensitiveDomain(domain), Rule: rule, ClientIP: clientIP, Count: 1, FirstAt: now, LastAt: now})
}

func (r *embeddedTrafficRegistry) drainAccess(nodeID int, uidByUUID map[string]int) []panel.SensitiveAccessEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	var result []panel.SensitiveAccessEvent
	for uuid, uid := range uidByUUID {
		key := embeddedTrafficKey{NodeID: nodeID, UUID: uuid}
		events := r.access[key]
		if len(events) == 0 {
			continue
		}
		for _, event := range events {
			event.UserID = uid
			result = append(result, event)
		}
		delete(r.access, key)
	}
	return result
}

func parseEmbeddedSensitiveRule(raw string) (embeddedSensitiveRule, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return embeddedSensitiveRule{}, false
	}
	kind := "suffix"
	pattern := raw
	if idx := strings.Index(raw, ":"); idx > 0 {
		kind = strings.ToLower(strings.TrimSpace(raw[:idx]))
		pattern = strings.TrimSpace(raw[idx+1:])
	}
	pattern = normalizeEmbeddedSensitiveDomain(pattern)
	if pattern == "" {
		return embeddedSensitiveRule{}, false
	}
	switch kind {
	case "domain", "suffix", "keyword":
		return embeddedSensitiveRule{Raw: raw, Kind: kind, Pattern: pattern}, true
	default:
		return embeddedSensitiveRule{}, false
	}
}

func normalizeEmbeddedSensitiveDomain(domain string) string {
	domain = strings.ToLower(strings.TrimSpace(domain))
	if host, _, err := net.SplitHostPort(domain); err == nil {
		domain = host
	}
	domain = strings.TrimSuffix(domain, ".")
	return domain
}
