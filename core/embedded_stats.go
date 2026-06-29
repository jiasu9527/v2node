package core

import "sync"

type embeddedTrafficKey struct {
	NodeID int
	UUID   string
}

type embeddedTrafficRegistry struct {
	mu       sync.Mutex
	traffic  map[embeddedTrafficKey]externalTrafficSnapshot
	clientIP map[embeddedTrafficKey]string
}

var globalEmbeddedTraffic = &embeddedTrafficRegistry{traffic: make(map[embeddedTrafficKey]externalTrafficSnapshot), clientIP: make(map[embeddedTrafficKey]string)}

type embeddedTrafficObserver struct {
	nodeID int
}

func (o embeddedTrafficObserver) AddTraffic(uuid string, upload int64, download int64, clientIP string) {
	if o.nodeID <= 0 || uuid == "" || upload+download <= 0 {
		return
	}
	globalEmbeddedTraffic.add(o.nodeID, uuid, upload, download, clientIP)
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
