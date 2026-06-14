package dispatcher

import "testing"

func TestSensitiveAuditRecordsMatchingDomain(t *testing.T) {
	d := &DefaultDispatcher{}
	d.ConfigureSensitiveAudit("inbound-a", true, []string{"suffix:example.com", "keyword:blocked"}, true)

	d.RecordSensitiveAccess("inbound-a", "user@example", "203.0.113.8", "www.example.com")
	d.RecordSensitiveAccess("inbound-a", "user@example", "203.0.113.8", "safe.example.net")
	d.RecordSensitiveAccess("inbound-a", "user@example", "203.0.113.8", "blocked-site.test")

	events := d.DrainSensitiveAccess("inbound-a")
	if len(events) != 2 {
		t.Fatalf("expected 2 sensitive events, got %#v", events)
	}
	if events[0].Email != "user@example" || events[0].Rule == "" || events[0].Domain == "" || events[0].Count != 1 {
		t.Fatalf("unexpected first event: %#v", events[0])
	}
	if len(d.DrainSensitiveAccess("inbound-a")) != 0 {
		t.Fatal("expected drain to clear events")
	}
}

func TestSensitiveAuditAggregatesSameUserDomainRule(t *testing.T) {
	d := &DefaultDispatcher{}
	d.ConfigureSensitiveAudit("inbound-a", true, []string{"domain:example.com"}, false)

	d.RecordSensitiveAccess("inbound-a", "user@example", "203.0.113.8", "example.com")
	d.RecordSensitiveAccess("inbound-a", "user@example", "203.0.113.9", "example.com")

	events := d.DrainSensitiveAccess("inbound-a")
	if len(events) != 1 || events[0].Count != 2 {
		t.Fatalf("expected one aggregated event, got %#v", events)
	}
	if events[0].ClientIP != "" {
		t.Fatalf("expected client ip hidden, got %#v", events[0])
	}
}
