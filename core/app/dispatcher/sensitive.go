package dispatcher

import (
	"context"
	"strings"
	"sync/atomic"
	"time"

	"github.com/xtls/xray-core/common/session"
)

type SensitiveAccessEvent struct {
	Tag      string
	Email    string
	Domain   string
	Rule     string
	ClientIP string
	Count    int64
	FirstAt  int64
	LastAt   int64
}

type sensitiveAuditRule struct {
	Raw     string
	Kind    string
	Pattern string
}

type sensitiveAuditConfig struct {
	Enabled     bool
	Rules       []sensitiveAuditRule
	LogClientIP bool
}

type sensitiveAccessBucket struct {
	Tag      string
	Email    string
	Domain   string
	Rule     string
	ClientIP string
	Count    atomic.Int64
	FirstAt  int64
	LastAt   atomic.Int64
}

func (d *DefaultDispatcher) ConfigureSensitiveAudit(tag string, enable bool, rules []string, logClientIP bool) {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return
	}
	if !enable || len(rules) == 0 {
		d.SensitiveAudits.Delete(tag)
		d.DrainSensitiveAccess(tag)
		return
	}
	parsed := make([]sensitiveAuditRule, 0, len(rules))
	for _, raw := range rules {
		if rule, ok := parseSensitiveAuditRule(raw); ok {
			parsed = append(parsed, rule)
		}
	}
	if len(parsed) == 0 {
		d.SensitiveAudits.Delete(tag)
		d.DrainSensitiveAccess(tag)
		return
	}
	d.SensitiveAudits.Store(tag, &sensitiveAuditConfig{Enabled: true, Rules: parsed, LogClientIP: logClientIP})
}

func (d *DefaultDispatcher) RecordSensitiveAccess(tag, email, clientIP, domain string) {
	tag = strings.TrimSpace(tag)
	email = strings.TrimSpace(email)
	domain = normalizeSensitiveDomain(domain)
	if tag == "" || email == "" || domain == "" {
		return
	}
	auditValue, ok := d.SensitiveAudits.Load(tag)
	if !ok {
		return
	}
	cfg, ok := auditValue.(*sensitiveAuditConfig)
	if !ok || cfg == nil || !cfg.Enabled {
		return
	}
	rule, ok := matchSensitiveAuditRule(cfg.Rules, domain)
	if !ok {
		return
	}
	if !cfg.LogClientIP {
		clientIP = ""
	}
	now := time.Now().Unix()
	key := strings.Join([]string{tag, email, domain, rule.Raw, clientIP}, "\x00")
	bucket := &sensitiveAccessBucket{Tag: tag, Email: email, Domain: domain, Rule: rule.Raw, ClientIP: clientIP, FirstAt: now}
	actual, loaded := d.SensitiveEvents.LoadOrStore(key, bucket)
	if loaded {
		bucket = actual.(*sensitiveAccessBucket)
	}
	bucket.Count.Add(1)
	bucket.LastAt.Store(now)
}

func (d *DefaultDispatcher) RecordSensitiveAccessFromContext(ctx context.Context, domain string) {
	inbound := session.InboundFromContext(ctx)
	if inbound == nil || inbound.User == nil || inbound.User.Email == "" {
		return
	}
	clientIP := ""
	clientIP = inbound.Source.Address.IP().String()
	d.RecordSensitiveAccess(inbound.Tag, inbound.User.Email, clientIP, domain)
}

func (d *DefaultDispatcher) DrainSensitiveAccess(tag string) []SensitiveAccessEvent {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return nil
	}
	result := make([]SensitiveAccessEvent, 0)
	d.SensitiveEvents.Range(func(key, value interface{}) bool {
		bucket, ok := value.(*sensitiveAccessBucket)
		if !ok || bucket == nil || bucket.Tag != tag {
			return true
		}
		d.SensitiveEvents.Delete(key)
		count := bucket.Count.Load()
		if count <= 0 {
			return true
		}
		lastAt := bucket.LastAt.Load()
		if lastAt <= 0 {
			lastAt = bucket.FirstAt
		}
		result = append(result, SensitiveAccessEvent{
			Tag:      bucket.Tag,
			Email:    bucket.Email,
			Domain:   bucket.Domain,
			Rule:     bucket.Rule,
			ClientIP: bucket.ClientIP,
			Count:    count,
			FirstAt:  bucket.FirstAt,
			LastAt:   lastAt,
		})
		return true
	})
	return result
}

func parseSensitiveAuditRule(raw string) (sensitiveAuditRule, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return sensitiveAuditRule{}, false
	}
	kind := "suffix"
	pattern := raw
	if idx := strings.Index(raw, ":"); idx > 0 {
		kind = strings.ToLower(strings.TrimSpace(raw[:idx]))
		pattern = strings.TrimSpace(raw[idx+1:])
	}
	pattern = normalizeSensitiveDomain(pattern)
	if pattern == "" {
		return sensitiveAuditRule{}, false
	}
	switch kind {
	case "domain", "suffix", "keyword":
		return sensitiveAuditRule{Raw: raw, Kind: kind, Pattern: pattern}, true
	default:
		return sensitiveAuditRule{}, false
	}
}

func matchSensitiveAuditRule(rules []sensitiveAuditRule, domain string) (sensitiveAuditRule, bool) {
	domain = normalizeSensitiveDomain(domain)
	for _, rule := range rules {
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
	return sensitiveAuditRule{}, false
}

func normalizeSensitiveDomain(domain string) string {
	domain = strings.ToLower(strings.TrimSpace(domain))
	domain = strings.TrimSuffix(domain, ".")
	return domain
}
