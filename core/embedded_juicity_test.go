package core

import (
	"testing"

	panel "github.com/wyx2685/v2node/api/v2board"
)

func TestBuildJuicityServerOptionsForEmbeddedServer(t *testing.T) {
	node := &panel.NodeInfo{Id: 7, Type: "juicity", Common: &panel.CommonNode{Protocol: "juicity", ServerPort: 443, ListenIP: "127.0.0.1", CongestionControl: "bbr", TlsSettings: panel.TlsSettings{CertFile: "/tmp/cert.pem", KeyFile: "/tmp/key.pem"}}}
	opts, listen, err := buildJuicityServerOptions(node, []panel.UserInfo{{Id: 1, Uuid: "11111111-1111-1111-1111-111111111111"}})
	if err != nil {
		t.Fatalf("buildJuicityServerOptions() error = %v", err)
	}
	if listen != "127.0.0.1:443" {
		t.Fatalf("listen = %q", listen)
	}
	if opts.Users["11111111-1111-1111-1111-111111111111"] != "11111111-1111-1111-1111-111111111111" {
		t.Fatalf("unexpected users: %#v", opts.Users)
	}
	if opts.Certificate != "/tmp/cert.pem" || opts.PrivateKey != "/tmp/key.pem" || opts.CongestionControl != "bbr" {
		t.Fatalf("unexpected options: %#v", opts)
	}
}
