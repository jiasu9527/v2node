package core

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	panel "github.com/wyx2685/v2node/api/v2board"
)

const defaultExternalConfigDir = "/etc/v2node"

type ExternalProcess struct {
	Protocol   string
	NodeID     int
	ConfigPath string
	Command    string
	Args       []string
	cmd        *exec.Cmd
}

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
		"listen":             fmt.Sprintf(":%d", node.Common.ServerPort),
		"certificate":        node.Common.CertInfoFile(),
		"private_key":        node.Common.CertKeyFile(),
		"congestion_control": firstNonEmpty(node.Common.CongestionControl, "bbr"),
		"users":              userMap,
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
	cfg := map[string]any{
		"portBindings": []map[string]any{{
			"port":         node.Common.ServerPort,
			"protocol":     transport,
			"mtu":          mtu,
			"multiplexing": node.Common.Multiplexing,
		}},
		"users": userList,
	}
	return json.MarshalIndent(cfg, "", "  ")
}

func NewExternalProcess(node *panel.NodeInfo, users []panel.UserInfo) (*ExternalProcess, error) {
	if node == nil || node.Common == nil {
		return nil, fmt.Errorf("missing external node config")
	}
	protocol := strings.ToLower(strings.TrimSpace(node.Type))
	var raw []byte
	var err error
	process := &ExternalProcess{Protocol: protocol, NodeID: node.Id}
	switch protocol {
	case "juicity":
		raw, err = RenderJuicityConfig(node, users)
		process.Command = "juicity-server"
		process.Args = []string{"run", "-c"}
	case "mieru":
		raw, err = RenderMieruConfig(node, users)
		process.Command = "mita"
		process.Args = []string{"run", "-c"}
	default:
		return nil, fmt.Errorf("unsupported external protocol: %s", protocol)
	}
	if err != nil {
		return nil, err
	}
	path := filepath.Join(externalConfigDir(), fmt.Sprintf("external-%s-%d.json", protocol, node.Id))
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, raw, 0600); err != nil {
		return nil, err
	}
	process.ConfigPath = path
	process.Args = append(process.Args, path)
	return process, nil
}

func (p *ExternalProcess) Start() error {
	if p == nil {
		return fmt.Errorf("nil external process")
	}
	cmd := exec.Command(p.Command, p.Args...)
	if err := cmd.Start(); err != nil {
		return err
	}
	p.cmd = cmd
	return nil
}

func (p *ExternalProcess) Stop() error {
	if p == nil || p.cmd == nil || p.cmd.Process == nil {
		return nil
	}
	return p.cmd.Process.Kill()
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
