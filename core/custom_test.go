package core

import (
	"testing"

	panel "github.com/wyx2685/v2node/api/v2board"
	"github.com/xtls/xray-core/app/proxyman"
	xrouter "github.com/xtls/xray-core/app/router"
	xcore "github.com/xtls/xray-core/core"
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

	outbound := findOutboundByTag(outbounds, "hk-direct")
	if outbound == nil {
		t.Fatalf("expected default_out outbound %q to exist", "hk-direct")
	}
	if got := outboundVia(t, outbound); got != "203.0.113.20" {
		t.Fatalf("default_out via = %q, want %q", got, "203.0.113.20")
	}

	if rule := findRouteByOutboundTag(routeConfig, "source-direct::"+info.Tag); rule != nil {
		t.Fatalf("unexpected auto source-direct rule when default_out is already defined")
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
