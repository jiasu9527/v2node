package dispatcher

import (
	"context"
	"testing"

	"github.com/xtls/xray-core/common/net"
	"github.com/xtls/xray-core/common/protocol"
	"github.com/xtls/xray-core/common/session"
)

func observationTestContext(tag, email string) context.Context {
	return session.ContextWithInbound(context.Background(), &session.Inbound{
		Tag:    tag,
		User:   &protocol.MemoryUser{Email: email},
		Source: net.TCPDestination(net.ParseAddress("203.0.113.9"), 40000),
	})
}

func TestObservationOptInAndDomainOrIP(t *testing.T) {
	d := new(DefaultDispatcher)
	ctx := observationTestContext("inbound-a", "user@forest.local")
	destination := net.TCPDestination(net.ParseAddress("192.0.2.10"), 443)
	d.RecordObservationFromContext(ctx, destination, "example.com")
	if events, _ := d.DrainObservation("inbound-a"); len(events) != 0 {
		t.Fatal("disabled observation recorded an event")
	}
	d.ConfigureObservationAudit("inbound-a", true)
	d.RecordObservationFromContext(ctx, destination, "WWW.Example.Com.")
	d.RecordObservationFromContext(ctx, destination, "")
	events, dropped := d.DrainObservation("inbound-a")
	if dropped != 0 || len(events) != 2 {
		t.Fatalf("events=%d dropped=%d", len(events), dropped)
	}
	if events[0].Domain != "www.example.com" || events[0].TargetIP != "192.0.2.10" || events[0].TargetPort != 443 || events[0].ClientIP != "203.0.113.9" || events[0].Result != "route_attempt" {
		t.Fatalf("wrong domain event: %#v", events[0])
	}
	if events[1].Domain != "" || events[1].TargetIP != "192.0.2.10" {
		t.Fatalf("wrong IP event: %#v", events[1])
	}
	if events[0].EventID == events[1].EventID || len(events[0].EventID) != 32 {
		t.Fatal("observation IDs are not unique 128-bit hex strings")
	}
	if remaining, _ := d.DrainObservation("inbound-a"); len(remaining) != 0 {
		t.Fatal("drain repeated events")
	}
	d.ConfigureObservationAudit("inbound-a", false)
	d.RecordObservationFromContext(ctx, destination, "again.example")
	if events, _ := d.DrainObservation("inbound-a"); len(events) != 0 {
		t.Fatal("disabled observation recorded after shutdown")
	}
}

func TestObservationRequiresAuthenticatedUser(t *testing.T) {
	d := new(DefaultDispatcher)
	d.ConfigureObservationAudit("inbound-a", true)
	d.RecordObservationFromContext(context.Background(), net.TCPDestination(net.ParseAddress("192.0.2.10"), 443), "example.com")
	if events, _ := d.DrainObservation("inbound-a"); len(events) != 0 {
		t.Fatal("anonymous request was observed")
	}
}
