package core

import (
	"testing"

	panel "github.com/wyx2685/v2node/api/v2board"
	"github.com/wyx2685/v2node/conf"
	"github.com/xtls/xray-core/app/proxyman"
	xrouter "github.com/xtls/xray-core/app/router"
	xcore "github.com/xtls/xray-core/core"
	xwireguard "github.com/xtls/xray-core/proxy/wireguard"
)

func TestGetCustomConfigCreatesSourceBoundDefaultOutbound(t *testing.T) {
	t.Parallel()

	info := &panel.NodeInfo{
		Tag: "[panel]-vless:1",
		Common: &panel.CommonNode{
			Protocol:    "vless",
			SendThrough: "203.0.113.10",
		},
	}

	_, outbounds, routeConfig, err := GetCustomConfig([]*panel.NodeInfo{info})
	if err != nil {
		t.Fatalf("GetCustomConfig() error = %v", err)
	}

	expectedTag := "source-direct::" + info.Tag
	outbound := findOutboundByTag(outbounds, expectedTag)
	if outbound == nil {
		t.Fatalf("expected source-bound outbound %q to exist", expectedTag)
	}

	if got := outboundVia(t, outbound); got != "203.0.113.10" {
		t.Fatalf("source-bound outbound via = %q, want %q", got, "203.0.113.10")
	}

	rule := findRouteByOutboundTag(routeConfig, expectedTag)
	if rule == nil {
		t.Fatalf("expected routing rule for outbound %q", expectedTag)
	}
	if len(rule.InboundTag) != 1 || rule.InboundTag[0] != info.Tag {
		t.Fatalf("routing rule inboundTag = %v, want [%q]", rule.InboundTag, info.Tag)
	}
}

func TestGetCustomConfigAppliesSendThroughToFreedomDefaultOut(t *testing.T) {
	t.Parallel()

	actionValue := `{"protocol":"freedom","tag":"hk-direct"}`
	info := &panel.NodeInfo{
		Tag: "[panel]-vless:2",
		Common: &panel.CommonNode{
			Protocol:    "vless",
			SendThrough: "203.0.113.20",
			Routes: []panel.Route{
				{
					Action:      "default_out",
					ActionValue: &actionValue,
				},
			},
		},
	}

	_, outbounds, routeConfig, err := GetCustomConfig([]*panel.NodeInfo{info})
	if err != nil {
		t.Fatalf("GetCustomConfig() error = %v", err)
	}

	expectedTag := scopedOutboundTag(info.Tag, "hk-direct")
	outbound := findOutboundByTag(outbounds, expectedTag)
	if outbound == nil {
		t.Fatalf("expected default_out outbound %q to exist", expectedTag)
	}
	if got := outboundVia(t, outbound); got != "203.0.113.20" {
		t.Fatalf("default_out via = %q, want %q", got, "203.0.113.20")
	}

	if rule := findRouteByOutboundTag(routeConfig, "source-direct::"+info.Tag); rule != nil {
		t.Fatalf("unexpected auto source-direct rule when default_out is already defined")
	}
}

func TestGetCustomConfigDefaultsSendThroughToInboundOrigin(t *testing.T) {
	t.Parallel()

	info := &panel.NodeInfo{
		Tag: "[panel]-vless:3",
		Common: &panel.CommonNode{
			Protocol: "vless",
			ListenIP: "0.0.0.0",
		},
	}

	_, outbounds, routeConfig, err := GetCustomConfig([]*panel.NodeInfo{info})
	if err != nil {
		t.Fatalf("GetCustomConfig() error = %v", err)
	}

	expectedTag := "source-direct::" + info.Tag
	outbound := findOutboundByTag(outbounds, expectedTag)
	if outbound == nil {
		t.Fatalf("expected source-bound outbound %q to exist", expectedTag)
	}
	if got := outboundVia(t, outbound); got != "origin" {
		t.Fatalf("source-bound outbound via = %q, want %q", got, "origin")
	}
	if rule := findRouteByOutboundTag(routeConfig, expectedTag); rule == nil {
		t.Fatalf("expected routing rule for outbound %q", expectedTag)
	}
}

func TestGetCustomConfigAppliesOriginSendThroughToProxyDefaultOut(t *testing.T) {
	t.Parallel()

	actionValue := `{
		"protocol":"shadowsocks",
		"tag":"hk-media",
		"settings":{
			"servers":[
				{
					"address":"198.51.100.8",
					"port":443,
					"method":"aes-128-gcm",
					"password":"secret"
				}
			]
		}
	}`
	info := &panel.NodeInfo{
		Tag: "[panel]-vless:4",
		Common: &panel.CommonNode{
			Protocol: "vless",
			ListenIP: "0.0.0.0",
			Routes: []panel.Route{
				{
					Action:      "default_out",
					ActionValue: &actionValue,
				},
			},
		},
	}

	_, outbounds, _, err := GetCustomConfig([]*panel.NodeInfo{info})
	if err != nil {
		t.Fatalf("GetCustomConfig() error = %v", err)
	}

	expectedTag := scopedOutboundTag(info.Tag, "hk-media")
	outbound := findOutboundByTag(outbounds, expectedTag)
	if outbound == nil {
		t.Fatalf("expected default_out outbound %q to exist", expectedTag)
	}
	if got := outboundVia(t, outbound); got != "origin" {
		t.Fatalf("default_out via = %q, want %q", got, "origin")
	}
}

func TestGetCustomConfigScopesCustomOutboundTagsPerNode(t *testing.T) {
	t.Parallel()

	actionValue := `{"protocol":"freedom","tag":"shared-direct"}`
	infos := []*panel.NodeInfo{
		{
			Tag: "[panel]-vless:11",
			Common: &panel.CommonNode{
				Protocol:    "vless",
				SendThrough: "203.0.113.11",
				Routes: []panel.Route{
					{
						Action:      "default_out",
						ActionValue: &actionValue,
					},
				},
			},
		},
		{
			Tag: "[panel]-vless:22",
			Common: &panel.CommonNode{
				Protocol:    "vless",
				SendThrough: "203.0.113.22",
				Routes: []panel.Route{
					{
						Action:      "default_out",
						ActionValue: &actionValue,
					},
				},
			},
		},
	}

	_, outbounds, routeConfig, err := GetCustomConfig(infos)
	if err != nil {
		t.Fatalf("GetCustomConfig() error = %v", err)
	}

	for _, info := range infos {
		expectedTag := scopedOutboundTag(info.Tag, "shared-direct")
		outbound := findOutboundByTag(outbounds, expectedTag)
		if outbound == nil {
			t.Fatalf("expected scoped outbound %q to exist", expectedTag)
		}
		if got := outboundVia(t, outbound); got != info.Common.SendThrough {
			t.Fatalf("outbound %q via = %q, want %q", expectedTag, got, info.Common.SendThrough)
		}
		rule := findRouteByOutboundTag(routeConfig, expectedTag)
		if rule == nil {
			t.Fatalf("expected routing rule for outbound %q", expectedTag)
		}
	}
}

func TestGetCoreBuildsWireGuardDefaultOutbound(t *testing.T) {
	t.Parallel()

	actionValue := `{
		"protocol":"wireguard",
		"tag":"hk-wireguard",
		"settings":{
			"secretKey":"uJv5tZMDltsiYEn+kUwb0Ll/CXWhMkaSCWWhfPEZM3A=",
			"address":["10.1.1.1"],
			"peers":[
				{
					"publicKey":"6e65ce0be17517110c17d77288ad87e7fd5252dcc7d09b95a39d61db03df832a",
					"endpoint":"127.0.0.1:1234"
				}
			]
		}
	}`
	info := &panel.NodeInfo{
		Tag: "[panel]-vless:33",
		Common: &panel.CommonNode{
			Protocol: "vless",
			Routes: []panel.Route{
				{
					Action:      "default_out",
					ActionValue: &actionValue,
				},
			},
		},
	}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("getCore() panic = %v", r)
		}
	}()

	instance := getCore(conf.New(), []*panel.NodeInfo{info})
	if instance == nil {
		t.Fatal("expected core instance")
	}
}

func TestGetCustomConfigNormalizesLegacyWireGuardKernelMode(t *testing.T) {
	t.Parallel()

	actionValue := `{
		"protocol":"wireguard",
		"tag":"hk-wireguard",
		"settings":{
			"secretKey":"uJv5tZMDltsiYEn+kUwb0Ll/CXWhMkaSCWWhfPEZM3A=",
			"address":["10.1.1.1"],
			"peers":[
				{
					"publicKey":"6e65ce0be17517110c17d77288ad87e7fd5252dcc7d09b95a39d61db03df832a",
					"endpoint":"127.0.0.1:1234"
				}
			],
			"kernelMode": false
		}
	}`
	info := &panel.NodeInfo{
		Tag: "[panel]-vless:44",
		Common: &panel.CommonNode{
			Protocol: "vless",
			Routes: []panel.Route{
				{
					Action:      "default_out",
					ActionValue: &actionValue,
				},
			},
		},
	}

	_, outbounds, _, err := GetCustomConfig([]*panel.NodeInfo{info})
	if err != nil {
		t.Fatalf("GetCustomConfig() error = %v", err)
	}

	expectedTag := scopedOutboundTag(info.Tag, "hk-wireguard")
	outbound := findOutboundByTag(outbounds, expectedTag)
	if outbound == nil {
		t.Fatalf("expected default_out outbound %q to exist", expectedTag)
	}

	instance := outboundProxyInstance(t, outbound)
	config, ok := instance.(*xwireguard.DeviceConfig)
	if !ok {
		t.Fatalf("proxy settings type = %T, want *wireguard.DeviceConfig", instance)
	}
	if !config.NoKernelTun {
		t.Fatalf("wireguard NoKernelTun = %v, want true", config.NoKernelTun)
	}
}

func findOutboundByTag(outbounds []*xcore.OutboundHandlerConfig, tag string) *xcore.OutboundHandlerConfig {
	for _, outbound := range outbounds {
		if outbound != nil && outbound.Tag == tag {
			return outbound
		}
	}
	return nil
}

func findRouteByOutboundTag(config *xrouter.Config, tag string) *xrouter.RoutingRule {
	if config == nil {
		return nil
	}
	for _, rule := range config.Rule {
		if rule != nil && rule.GetTag() == tag {
			return rule
		}
	}
	return nil
}

func outboundVia(t *testing.T, outbound *xcore.OutboundHandlerConfig) string {
	t.Helper()

	if outbound == nil || outbound.SenderSettings == nil {
		return ""
	}

	instance, err := outbound.SenderSettings.GetInstance()
	if err != nil {
		t.Fatalf("SenderSettings.GetInstance() error = %v", err)
	}
	senderConfig, ok := instance.(*proxyman.SenderConfig)
	if !ok {
		t.Fatalf("sender settings type = %T, want *proxyman.SenderConfig", instance)
	}
	if senderConfig.GetVia() == nil {
		return ""
	}
	return senderConfig.GetVia().AsAddress().String()
}

func outboundProxyInstance(t *testing.T, outbound *xcore.OutboundHandlerConfig) interface{} {
	t.Helper()

	if outbound == nil || outbound.ProxySettings == nil {
		t.Fatalf("outbound proxy settings are missing")
	}

	instance, err := outbound.ProxySettings.GetInstance()
	if err != nil {
		t.Fatalf("ProxySettings.GetInstance() error = %v", err)
	}
	return instance
}
