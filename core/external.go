package core

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	panel "github.com/wyx2685/v2node/api/v2board"
)

const defaultExternalConfigDir = "/etc/v2node"

func RenderJuicityConfig(node *panel.NodeInfo, users []panel.UserInfo) ([]byte, error) {
	if node == nil || node.Common == nil {
		return nil, fmt.Errorf("missing juicity node config")
	}
	userMap := make(map[string]string, len(users))
	for _, user := range users {
		if strings.TrimSpace(user.Uuid) == "" {
			continue
		}
		userMap[user.Uuid] = user.Uuid
	}
	cfg := map[string]any{
		"listen":              fmt.Sprintf(":%d", node.Common.ServerPort),
		"certificate":         node.Common.CertInfoFile(),
		"private_key":         node.Common.CertKeyFile(),
		"congestion_control":  firstNonEmpty(node.Common.CongestionControl, "bbr"),
		"users":               userMap,
		"v2node_observer_log": ExternalObservabilityLogPath(node),
	}
	return json.MarshalIndent(cfg, "", "  ")
}

func RenderMieruConfig(node *panel.NodeInfo, users []panel.UserInfo) ([]byte, error) {
	if node == nil || node.Common == nil {
		return nil, fmt.Errorf("missing mieru node config")
	}
	transport := strings.ToUpper(firstNonEmpty(node.Common.Transport, "TCP"))
	mtu := node.Common.MTU
	if mtu <= 0 {
		mtu = 1400
	}
	userList := make([]map[string]any, 0, len(users))
	for _, user := range users {
		if strings.TrimSpace(user.Uuid) == "" {
			continue
		}
		userList = append(userList, map[string]any{
			"name":     strconv.Itoa(user.Id),
			"password": user.Uuid,
		})
	}
	portBinding := map[string]any{
		"port":     node.Common.ServerPort,
		"protocol": transport,
	}
	if node.Common.Multiplexing != "" {
		portBinding["multiplexing"] = node.Common.Multiplexing
	}
	cfg := map[string]any{
		"portBindings": []map[string]any{portBinding},
		"users":        userList,
		"mtu":          mtu,
		"loggingLevel": "INFO",
	}
	return json.MarshalIndent(cfg, "", "  ")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func externalConfigDir() string {
	if value := strings.TrimSpace(os.Getenv("V2NODE_EXTERNAL_CONFIG_DIR")); value != "" {
		return value
	}
	return defaultExternalConfigDir
}
