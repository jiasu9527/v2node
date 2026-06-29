package core

import (
	"testing"

	panel "github.com/wyx2685/v2node/api/v2board"
)

func TestNewEmbeddedProtocolServerDoesNotRequireExternalBinary(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	for _, protocol := range []string{"juicity", "mieru", "naive"} {
		t.Run(protocol, func(t *testing.T) {
			node := &panel.NodeInfo{Id: 9, Type: protocol, Common: &panel.CommonNode{Protocol: protocol, ExternalProtocol: true, ServerPort: 0}}
			server, err := NewEmbeddedProtocolServer(node, []panel.UserInfo{{Id: 1, Uuid: "11111111-1111-1111-1111-111111111111"}})
			if err != nil {
				t.Fatalf("NewEmbeddedProtocolServer() error = %v", err)
			}
			if server == nil || server.Protocol() != protocol {
				t.Fatalf("unexpected embedded server: %#v", server)
			}
		})
	}
}
