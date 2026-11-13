package core

import (
	panel "github.com/wyx2685/v2node/api/v2board"
	"github.com/xtls/xray-core/app/proxyman"
	"testing"
)

func TestInboundSniffingPreservesProxyChainDestination(t *testing.T) {
	for _, protocol := range []string{"anytls", "trojan", "vless", "vmess"} {
		t.Run(protocol, func(t *testing.T) {
			cfg, err := buildInbound(&panel.NodeInfo{
				Type: protocol,
				Common: &panel.CommonNode{
					Protocol: protocol, ListenIP: "127.0.0.1", ServerPort: 14443,
					Network: "tcp",
				},
			}, "route-only-test")
			if err != nil {
				t.Fatal(err)
			}
			decoded, err := cfg.ReceiverSettings.GetInstance()
			if err != nil {
				t.Fatal(err)
			}
			receiver := decoded.(*proxyman.ReceiverConfig)
			sniff := receiver.SniffingSettings
			if sniff == nil || !sniff.Enabled || !sniff.RouteOnly {
				t.Fatalf("expected routing-only sniffing, got %+v", sniff)
			}
			if len(sniff.DestinationOverride) != 3 {
				t.Fatalf("lost domain sniffing protocols: %+v", sniff)
			}
		})
	}
}
