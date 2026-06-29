package core

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	mieruMetrics "github.com/enfein/mieru/v3/pkg/metrics"
	panel "github.com/wyx2685/v2node/api/v2board"
)

var ErrExternalTrafficUnsupported = errors.New("external protocol traffic statistics unsupported")

type externalTrafficSnapshot struct {
	Upload   int64
	Download int64
}

type externalObserverEvent struct {
	Type     string `json:"type"`
	UUID     string `json:"uuid"`
	User     string `json:"user"`
	UserID   int    `json:"user_id"`
	Upload   int64  `json:"upload"`
	Download int64  `json:"download"`
	ClientIP string `json:"client_ip"`
	Domain   string `json:"domain"`
	Rule     string `json:"rule"`
	Count    int64  `json:"count"`
	FirstAt  int64  `json:"first_at"`
	LastAt   int64  `json:"last_at"`
}

type ExternalTrafficCollector struct {
	protocol        string
	nodeID          int
	observerLogPath string
	prev            map[int]externalTrafficSnapshot
	onlinePrev      map[int]externalTrafficSnapshot
	observerOffset  int64
	observerEvents  []externalObserverEvent
	trafficCursor   int
	onlineCursor    int
	sensitiveCursor int
	mu              sync.Mutex
}

func NewExternalTrafficCollector(node *panel.NodeInfo) *ExternalTrafficCollector {
	protocol := ""
	nodeID := 0
	observerLogPath := ""
	if node != nil {
		nodeID = node.Id
		protocol = strings.ToLower(strings.TrimSpace(node.Type))
		if protocol == "" && node.Common != nil {
			protocol = strings.ToLower(strings.TrimSpace(node.Common.Protocol))
		}
		observerLogPath = ExternalObservabilityLogPath(node)
	}
	return &ExternalTrafficCollector{
		protocol:        protocol,
		nodeID:          nodeID,
		observerLogPath: observerLogPath,
		prev:            make(map[int]externalTrafficSnapshot),
		onlinePrev:      make(map[int]externalTrafficSnapshot),
	}
}

func ExternalObservabilityLogPath(node *panel.NodeInfo) string {
	protocol := "external"
	nodeID := 0
	if node != nil {
		nodeID = node.Id
		protocol = strings.ToLower(strings.TrimSpace(node.Type))
		if protocol == "" && node.Common != nil {
			protocol = strings.ToLower(strings.TrimSpace(node.Common.Protocol))
		}
	}
	return filepath.Join(externalConfigDir(), fmt.Sprintf("external-%s-%d.observe.jsonl", protocol, nodeID))
}

func (c *ExternalTrafficCollector) CollectOnlineUsers(users []panel.UserInfo, minTrafficKB int) ([]panel.OnlineUser, error) {
	if c == nil {
		return nil, ErrExternalTrafficUnsupported
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	var (
		snapshots map[int]externalTrafficSnapshot
		err       error
	)
	switch c.protocol {
	case "mieru":
		snapshots, err = c.collectMieruTraffic(users)
	case "juicity":
		snapshots = c.collectEmbeddedTraffic(users)
		if len(snapshots) > 0 {
			return c.deltaOnlineUsers(snapshots, minTrafficKB), nil
		}
		return c.collectObserverOnlineUsers(users, minTrafficKB)
	default:
		return nil, fmt.Errorf("%w: %s", ErrExternalTrafficUnsupported, c.protocol)
	}
	if err != nil {
		return nil, err
	}
	return c.deltaOnlineUsers(snapshots, minTrafficKB), nil
}

func (c *ExternalTrafficCollector) CollectSensitiveAccess(users []panel.UserInfo) ([]panel.SensitiveAccessEvent, error) {
	if c == nil {
		return nil, ErrExternalTrafficUnsupported
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.protocol != "juicity" {
		return nil, fmt.Errorf("%w: %s", ErrExternalTrafficUnsupported, c.protocol)
	}
	if err := c.loadObserverEvents(); err != nil {
		return nil, err
	}
	uidByUser := buildExternalUIDMap(users)
	events := make([]panel.SensitiveAccessEvent, 0)
	for c.sensitiveCursor < len(c.observerEvents) {
		event := c.observerEvents[c.sensitiveCursor]
		c.sensitiveCursor++
		if strings.ToLower(strings.TrimSpace(event.Type)) != "access" || strings.TrimSpace(event.Domain) == "" {
			continue
		}
		uid := externalEventUID(event, uidByUser)
		if uid <= 0 {
			continue
		}
		count := event.Count
		if count <= 0 {
			count = 1
		}
		events = append(events, panel.SensitiveAccessEvent{
			UserID:   uid,
			Domain:   event.Domain,
			Rule:     event.Rule,
			ClientIP: event.ClientIP,
			Count:    count,
			FirstAt:  event.FirstAt,
			LastAt:   event.LastAt,
		})
	}
	if len(events) == 0 {
		return nil, nil
	}
	return events, nil
}

func (c *ExternalTrafficCollector) CollectTraffic(users []panel.UserInfo, minTrafficKB int) ([]panel.UserTraffic, error) {
	if c == nil {
		return nil, ErrExternalTrafficUnsupported
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	var (
		snapshots map[int]externalTrafficSnapshot
		err       error
	)
	switch c.protocol {
	case "mieru":
		snapshots, err = c.collectMieruTraffic(users)
	case "juicity":
		snapshots = c.collectEmbeddedTraffic(users)
		if len(snapshots) > 0 {
			return c.deltaTraffic(snapshots, minTrafficKB), nil
		}
		return c.collectObserverTraffic(users, minTrafficKB)
	default:
		return nil, fmt.Errorf("%w: %s", ErrExternalTrafficUnsupported, c.protocol)
	}
	if err != nil {
		return nil, err
	}
	return c.deltaTraffic(snapshots, minTrafficKB), nil
}

func (c *ExternalTrafficCollector) deltaTraffic(snapshots map[int]externalTrafficSnapshot, minTrafficKB int) []panel.UserTraffic {
	if c.prev == nil {
		c.prev = make(map[int]externalTrafficSnapshot)
	}
	if len(c.prev) == 0 {
		c.prev = copyExternalTrafficSnapshots(snapshots)
		return nil
	}

	minBytes := int64(minTrafficKB) * 1000
	result := make([]panel.UserTraffic, 0, len(snapshots))
	uids := make([]int, 0, len(snapshots))
	for uid := range snapshots {
		uids = append(uids, uid)
	}
	sort.Ints(uids)
	seen := make(map[int]struct{}, len(snapshots))
	for _, uid := range uids {
		seen[uid] = struct{}{}
		current := snapshots[uid]
		previous, ok := c.prev[uid]
		if !ok || current.Upload < previous.Upload || current.Download < previous.Download {
			c.prev[uid] = current
			continue
		}
		up := current.Upload - previous.Upload
		down := current.Download - previous.Download
		if up+down > minBytes {
			result = append(result, panel.UserTraffic{UID: uid, Upload: up, Download: down})
			c.prev[uid] = current
		}
	}
	for uid := range c.prev {
		if _, ok := seen[uid]; !ok {
			delete(c.prev, uid)
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func (c *ExternalTrafficCollector) deltaOnlineUsers(snapshots map[int]externalTrafficSnapshot, minTrafficKB int) []panel.OnlineUser {
	if c.onlinePrev == nil {
		c.onlinePrev = make(map[int]externalTrafficSnapshot)
	}
	baseline := c.onlinePrev
	if len(baseline) == 0 {
		baseline = c.prev
	}
	if len(baseline) == 0 {
		c.onlinePrev = copyExternalTrafficSnapshots(snapshots)
		return nil
	}

	minBytes := int64(minTrafficKB) * 1000
	uids := make([]int, 0, len(snapshots))
	for uid := range snapshots {
		uids = append(uids, uid)
	}
	sort.Ints(uids)
	result := make([]panel.OnlineUser, 0, len(uids))
	for _, uid := range uids {
		current := snapshots[uid]
		previous, ok := baseline[uid]
		if !ok || current.Upload < previous.Upload || current.Download < previous.Download {
			continue
		}
		if current.Upload-previous.Upload+current.Download-previous.Download > minBytes {
			result = append(result, panel.OnlineUser{UID: uid, IP: "external:" + c.protocol})
		}
	}
	c.onlinePrev = copyExternalTrafficSnapshots(snapshots)
	if len(result) == 0 {
		return nil
	}
	return result
}

func copyExternalTrafficSnapshots(in map[int]externalTrafficSnapshot) map[int]externalTrafficSnapshot {
	out := make(map[int]externalTrafficSnapshot, len(in))
	for uid, snapshot := range in {
		out[uid] = snapshot
	}
	return out
}

func (c *ExternalTrafficCollector) collectEmbeddedTraffic(users []panel.UserInfo) map[int]externalTrafficSnapshot {
	uuidByUID := make(map[int]string, len(users))
	for _, user := range users {
		if strings.TrimSpace(user.Uuid) != "" {
			uuidByUID[user.Id] = user.Uuid
		}
	}
	return globalEmbeddedTraffic.snapshot(c.nodeID, uuidByUID)
}

func (c *ExternalTrafficCollector) collectMieruTraffic(users []panel.UserInfo) (map[int]externalTrafficSnapshot, error) {
	result := make(map[int]externalTrafficSnapshot)
	for _, user := range users {
		uidName := strconv.Itoa(user.Id)
		group := mieruMetrics.GetMetricGroupByName(fmt.Sprintf(mieruMetrics.UserMetricGroupFormat, uidName))
		if group == nil {
			continue
		}
		uploadMetric, okUpload := group.GetMetric(mieruMetrics.UserMetricUploadBytes)
		downloadMetric, okDownload := group.GetMetric(mieruMetrics.UserMetricDownloadBytes)
		if !okUpload && !okDownload {
			continue
		}
		snapshot := externalTrafficSnapshot{}
		if counter, ok := uploadMetric.(*mieruMetrics.Counter); ok {
			snapshot.Upload = counter.Load()
		}
		if counter, ok := downloadMetric.(*mieruMetrics.Counter); ok {
			snapshot.Download = counter.Load()
		}
		result[user.Id] = snapshot
	}
	return result, nil
}

func ParseMieruMetricsTraffic(raw []byte, users []panel.UserInfo) (map[int]externalTrafficSnapshot, error) {
	body, err := extractJSONObject(raw)
	if err != nil {
		return nil, err
	}
	var metrics struct {
		Users map[string]map[string]int64 `json:"users"`
	}
	if err := json.Unmarshal(body, &metrics); err != nil {
		return nil, fmt.Errorf("decode mieru metrics: %w", err)
	}
	if len(metrics.Users) == 0 {
		return nil, nil
	}
	nameToUID := make(map[string]int, len(users)*2)
	for _, user := range users {
		nameToUID[strconv.Itoa(user.Id)] = user.Id
		if strings.TrimSpace(user.Uuid) != "" {
			nameToUID[user.Uuid] = user.Id
		}
	}
	result := make(map[int]externalTrafficSnapshot)
	for name, values := range metrics.Users {
		uid, ok := nameToUID[name]
		if !ok {
			if parsed, err := strconv.Atoi(name); err == nil {
				uid = parsed
				ok = true
			}
		}
		if !ok || uid <= 0 {
			continue
		}
		result[uid] = externalTrafficSnapshot{
			Upload:   values["UploadBytes"],
			Download: values["DownloadBytes"],
		}
	}
	if len(result) == 0 {
		return nil, nil
	}
	return result, nil
}

func (c *ExternalTrafficCollector) collectObserverTraffic(users []panel.UserInfo, minTrafficKB int) ([]panel.UserTraffic, error) {
	if err := c.loadObserverEvents(); err != nil {
		return nil, err
	}
	uidByUser := buildExternalUIDMap(users)
	byUID := make(map[int]externalTrafficSnapshot)
	for c.trafficCursor < len(c.observerEvents) {
		event := c.observerEvents[c.trafficCursor]
		c.trafficCursor++
		if strings.ToLower(strings.TrimSpace(event.Type)) != "traffic" {
			continue
		}
		uid := externalEventUID(event, uidByUser)
		if uid <= 0 {
			continue
		}
		snapshot := byUID[uid]
		snapshot.Upload += event.Upload
		snapshot.Download += event.Download
		byUID[uid] = snapshot
	}
	minBytes := int64(minTrafficKB) * 1000
	result := make([]panel.UserTraffic, 0, len(byUID))
	uids := make([]int, 0, len(byUID))
	for uid := range byUID {
		uids = append(uids, uid)
	}
	sort.Ints(uids)
	for _, uid := range uids {
		snapshot := byUID[uid]
		if snapshot.Upload+snapshot.Download > minBytes {
			result = append(result, panel.UserTraffic{UID: uid, Upload: snapshot.Upload, Download: snapshot.Download})
		}
	}
	if len(result) == 0 {
		return nil, nil
	}
	return result, nil
}

func (c *ExternalTrafficCollector) collectObserverOnlineUsers(users []panel.UserInfo, minTrafficKB int) ([]panel.OnlineUser, error) {
	if err := c.loadObserverEvents(); err != nil {
		return nil, err
	}
	uidByUser := buildExternalUIDMap(users)
	minBytes := int64(minTrafficKB) * 1000
	seen := make(map[int]map[string]struct{})
	for c.onlineCursor < len(c.observerEvents) {
		event := c.observerEvents[c.onlineCursor]
		c.onlineCursor++
		uid := externalEventUID(event, uidByUser)
		if uid <= 0 {
			continue
		}
		ip := strings.TrimSpace(event.ClientIP)
		if ip == "" {
			ip = "external:" + c.protocol
		}
		typeName := strings.ToLower(strings.TrimSpace(event.Type))
		if typeName == "traffic" && event.Upload+event.Download <= minBytes {
			continue
		}
		if typeName != "traffic" && typeName != "access" {
			continue
		}
		if seen[uid] == nil {
			seen[uid] = make(map[string]struct{})
		}
		seen[uid][ip] = struct{}{}
	}
	if len(seen) == 0 {
		return nil, nil
	}
	uids := make([]int, 0, len(seen))
	for uid := range seen {
		uids = append(uids, uid)
	}
	sort.Ints(uids)
	result := make([]panel.OnlineUser, 0)
	for _, uid := range uids {
		ips := make([]string, 0, len(seen[uid]))
		for ip := range seen[uid] {
			ips = append(ips, ip)
		}
		sort.Strings(ips)
		for _, ip := range ips {
			result = append(result, panel.OnlineUser{UID: uid, IP: ip})
		}
	}
	return result, nil
}

func (c *ExternalTrafficCollector) loadObserverEvents() error {
	path := strings.TrimSpace(c.observerLogPath)
	if path == "" {
		path = filepath.Join(externalConfigDir(), fmt.Sprintf("external-%s-%d.observe.jsonl", c.protocol, c.nodeID))
	}
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if info.Size() < c.observerOffset {
		c.observerOffset = 0
		c.observerEvents = nil
		c.trafficCursor = 0
		c.onlineCursor = 0
		c.sensitiveCursor = 0
	}
	if _, err := file.Seek(c.observerOffset, io.SeekStart); err != nil {
		return err
	}
	reader := bufio.NewReader(file)
	for {
		line, err := reader.ReadString('\n')
		if len(strings.TrimSpace(line)) > 0 {
			var event externalObserverEvent
			if jsonErr := json.Unmarshal([]byte(strings.TrimSpace(line)), &event); jsonErr == nil {
				c.observerEvents = append(c.observerEvents, event)
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return err
		}
	}
	offset, err := file.Seek(0, io.SeekCurrent)
	if err == nil {
		c.observerOffset = offset
	}
	return nil
}

func buildExternalUIDMap(users []panel.UserInfo) map[string]int {
	result := make(map[string]int, len(users)*2)
	for _, user := range users {
		result[strconv.Itoa(user.Id)] = user.Id
		if strings.TrimSpace(user.Uuid) != "" {
			result[user.Uuid] = user.Id
		}
	}
	return result
}

func externalEventUID(event externalObserverEvent, uidByUser map[string]int) int {
	if event.UserID > 0 {
		return event.UserID
	}
	for _, key := range []string{event.UUID, event.User} {
		if uid, ok := uidByUser[strings.TrimSpace(key)]; ok {
			return uid
		}
	}
	return 0
}

func extractJSONObject(raw []byte) ([]byte, error) {
	text := strings.TrimSpace(string(raw))
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start < 0 || end < start {
		return nil, fmt.Errorf("metrics output does not contain json object")
	}
	return []byte(text[start : end+1]), nil
}
