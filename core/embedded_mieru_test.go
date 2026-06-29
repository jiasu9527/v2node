package core

import (
	"testing"

	panel "github.com/wyx2685/v2node/api/v2board"
)

func TestBuildMieruServerConfigForEmbeddedServer(t *testing.T) {
	node := &panel.NodeInfo{Id: 8, Type: "mieru", Common: &panel.CommonNode{Protocol: "mieru", ServerPort: 2999, Transport: "UDP", MTU: 1280}}
	cfg, err := buildMieruServerConfig(node, []panel.UserInfo{{Id: 1001, Uuid: "uuid-password"}})
	if err != nil {
		t.Fatalf("buildMieruServerConfig() error = %v", err)
	}
	if cfg.GetMtu() != 1280 {
		t.Fatalf("mtu = %d, want 1280", cfg.GetMtu())
	}
	bindings := cfg.GetPortBindings()
	if len(bindings) != 1 || bindings[0].GetPort() != 2999 || bindings[0].GetProtocol().String() != "UDP" {
		t.Fatalf("unexpected port bindings: %#v", bindings)
	}
	users := cfg.GetUsers()
	if len(users) != 1 || users[0].GetName() != "1001" || users[0].GetPassword() != "uuid-password" {
		t.Fatalf("unexpected users: %#v", users)
	}
}
