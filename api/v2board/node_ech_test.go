package panel

import (
	"encoding/json"
	"testing"
)

func TestCommonNodeUnmarshalPreservesECHSettings(t *testing.T) {
	t.Parallel()

	var node CommonNode
	raw := []byte(`{
		"protocol":"vless",
		"tls":1,
		"tls_settings":{
			"server_name":"inner.example.com",
			"ech":"custom",
			"ech_server_name":"cover.example.com",
			"ech_key":"ZWNoLWtleQ==",
			"ech_config":"ZWNoLWNvbmZpZw=="
		}
	}`)
	if err := json.Unmarshal(raw, &node); err != nil {
		t.Fatalf("unmarshal common node: %v", err)
	}
	if node.TlsSettings.Ech != "custom" {
		t.Fatalf("TlsSettings.Ech = %q, want %q", node.TlsSettings.Ech, "custom")
	}
	if node.TlsSettings.EchServerName != "cover.example.com" {
		t.Fatalf("TlsSettings.EchServerName = %q, want %q", node.TlsSettings.EchServerName, "cover.example.com")
	}
	if node.TlsSettings.EchKey != "ZWNoLWtleQ==" {
		t.Fatalf("TlsSettings.EchKey = %q, want preserved base64", node.TlsSettings.EchKey)
	}
	if node.TlsSettings.EchConfig != "ZWNoLWNvbmZpZw==" {
		t.Fatalf("TlsSettings.EchConfig = %q, want preserved base64", node.TlsSettings.EchConfig)
	}
}
