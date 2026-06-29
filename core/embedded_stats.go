package core

import (
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
	nodeID int
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
	globalEmbeddedTraffic.addAccess(o.nodeID, uuid, domain, network, clientIP)
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

func (r *embeddedTrafficRegistry) addAccess(nodeID int, uuid string, domain string, network string, clientIP string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := embeddedTrafficKey{NodeID: nodeID, UUID: uuid}
	now := time.Now().Unix()
	r.access[key] = append(r.access[key], panel.SensitiveAccessEvent{Domain: domain, Rule: "embedded:" + network, ClientIP: clientIP, Count: 1, FirstAt: now, LastAt: now})
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
