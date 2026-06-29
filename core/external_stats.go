package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"

	panel "github.com/wyx2685/v2node/api/v2board"
)

var ErrExternalTrafficUnsupported = errors.New("external protocol traffic statistics unsupported")

type externalTrafficSnapshot struct {
	Upload   int64
	Download int64
}

type ExternalTrafficCollector struct {
	protocol   string
	prev       map[int]externalTrafficSnapshot
	onlinePrev map[int]externalTrafficSnapshot
	mu         sync.Mutex
}

func NewExternalTrafficCollector(node *panel.NodeInfo) *ExternalTrafficCollector {
	protocol := ""
	if node != nil {
		protocol = strings.ToLower(strings.TrimSpace(node.Type))
		if protocol == "" && node.Common != nil {
			protocol = strings.ToLower(strings.TrimSpace(node.Common.Protocol))
		}
	}
	return &ExternalTrafficCollector{
		protocol:   protocol,
		prev:       make(map[int]externalTrafficSnapshot),
		onlinePrev: make(map[int]externalTrafficSnapshot),
	}
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
		return nil, fmt.Errorf("%w: juicity-server does not expose online user counters", ErrExternalTrafficUnsupported)
	default:
		return nil, fmt.Errorf("%w: %s", ErrExternalTrafficUnsupported, c.protocol)
	}
	if err != nil {
		return nil, err
	}
	return c.deltaOnlineUsers(snapshots, minTrafficKB), nil
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
		return nil, fmt.Errorf("%w: juicity-server does not expose per-user traffic counters", ErrExternalTrafficUnsupported)
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

func (c *ExternalTrafficCollector) collectMieruTraffic(users []panel.UserInfo) (map[int]externalTrafficSnapshot, error) {
	output, err := exec.Command("mita", "get", "metrics").CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("mita get metrics failed: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return ParseMieruMetricsTraffic(output, users)
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

func extractJSONObject(raw []byte) ([]byte, error) {
	text := strings.TrimSpace(string(raw))
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start < 0 || end < start {
		return nil, fmt.Errorf("metrics output does not contain json object")
	}
	return []byte(text[start : end+1]), nil
}
