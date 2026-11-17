package dispatcher

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/xtls/xray-core/common/net"
	"github.com/xtls/xray-core/common/session"
)

// ObservationEvent describes a routed destination attempt, not a confirmed
// connection or page view. Content and full URLs are intentionally absent.
type ObservationEvent struct {
	EventID    string
	Tag        string
	Email      string
	Domain     string
	TargetIP   string
	TargetPort int
	Network    string
	ClientIP   string
	StartedAt  int64
	Result     string
}

const maxObservationEvents = 8192

type observationAuditState struct {
	mu      sync.Mutex
	events  []ObservationEvent
	dropped uint64
}

func (d *DefaultDispatcher) ConfigureObservationAudit(tag string, enable bool) {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return
	}
	if !enable {
		d.ObservationAudits.Delete(tag)
		return
	}
	d.ObservationAudits.LoadOrStore(tag, &observationAuditState{})
}

// RecordObservationFromContext is called after the inbound user has been
// authenticated and the destination has been determined (including sniffing).
// A route attempt alone does not establish that the remote site was reached.
func (d *DefaultDispatcher) RecordObservationFromContext(ctx context.Context, destination net.Destination, sniffedDomain string) {
	inbound := session.InboundFromContext(ctx)
	if inbound == nil || inbound.User == nil || inbound.User.Email == "" {
		return
	}
	value, ok := d.ObservationAudits.Load(inbound.Tag)
	if !ok {
		return
	}
	state, ok := value.(*observationAuditState)
	if !ok || state == nil {
		return
	}
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return
	}
	event := ObservationEvent{
		EventID:    hex.EncodeToString(random[:]),
		Tag:        inbound.Tag,
		Email:      inbound.User.Email,
		Domain:     normalizeObservationHost(sniffedDomain),
		TargetPort: int(destination.Port),
		Network:    strings.ToLower(destination.Network.String()),
		StartedAt:  time.Now().Unix(),
		Result:     "route_attempt",
	}
	if event.Domain == "" && destination.Address.Family().IsDomain() {
		event.Domain = normalizeObservationHost(destination.Address.Domain())
	}
	if destination.Address.Family().IsIP() {
		event.TargetIP = destination.Address.IP().String()
	}
	if inbound.Source.Address != nil && inbound.Source.Address.Family().IsIP() {
		event.ClientIP = inbound.Source.Address.IP().String()
	}
	if event.Domain == "" && event.TargetIP == "" {
		return
	}
	state.mu.Lock()
	if len(state.events) >= maxObservationEvents {
		copy(state.events, state.events[1:])
		state.events[len(state.events)-1] = event
		state.dropped++
	} else {
		state.events = append(state.events, event)
	}
	state.mu.Unlock()
}

func normalizeObservationHost(host string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	// HTTP Host can include a port; the panel stores the host and port in
	// separate fields and rejects a colon in target_host.
	if index := strings.LastIndexByte(host, ':'); index > 0 && strings.Count(host, ":") == 1 {
		if _, err := strconv.ParseUint(host[index+1:], 10, 16); err == nil {
			host = host[:index]
		}
	}
	host = strings.TrimSuffix(host, ".")
	if len(host) > 253 || strings.ContainsAny(host, "/:@?# \\ \t\r\n") || strings.Contains(host, "..") {
		return ""
	}
	return host
}

func (d *DefaultDispatcher) DrainObservation(tag string) ([]ObservationEvent, uint64) {
	value, ok := d.ObservationAudits.Load(strings.TrimSpace(tag))
	if !ok {
		return nil, 0
	}
	state, ok := value.(*observationAuditState)
	if !ok || state == nil {
		return nil, 0
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	events := state.events
	dropped := state.dropped
	state.events = nil
	state.dropped = 0
	return events, dropped
}
