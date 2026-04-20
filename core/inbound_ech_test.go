package core

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"

	panel "github.com/wyx2685/v2node/api/v2board"
	proxyman "github.com/xtls/xray-core/app/proxyman"
	xtlstls "github.com/xtls/xray-core/transport/internet/tls"
)

func TestBuildInboundAppliesECHServerKeys(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	certFile := filepath.Join(dir, "test-cert.pem")
	keyFile := filepath.Join(dir, "test-key.pem")
	if err := os.WriteFile(certFile, []byte("cert"), 0600); err != nil {
		t.Fatalf("write cert file: %v", err)
	}
	if err := os.WriteFile(keyFile, []byte("key"), 0600); err != nil {
		t.Fatalf("write key file: %v", err)
	}

	rawECHKeys := []byte("ech-server-keys")
	config, err := buildInbound(&panel.NodeInfo{
		Type:     "vless",
		Security: panel.Tls,
		Common: &panel.CommonNode{
			Protocol:   "vless",
			ListenIP:   "0.0.0.0",
			ServerPort: 443,
			TlsSettings: panel.TlsSettings{
				ServerName: "inner.example.com",
				Ech:        "custom",
				EchKey:     base64.StdEncoding.EncodeToString(rawECHKeys),
			},
			CertInfo: &panel.CertInfo{
				CertMode: "file",
				CertFile: certFile,
				KeyFile:  keyFile,
			},
		},
	}, "test-tag")
	if err != nil {
		t.Fatalf("buildInbound() error = %v", err)
	}

	receiverMessage, err := config.ReceiverSettings.GetInstance()
	if err != nil {
		t.Fatalf("decode receiver settings: %v", err)
	}
	receiver, ok := receiverMessage.(*proxyman.ReceiverConfig)
	if !ok {
		t.Fatalf("receiver settings type = %T, want *proxyman.ReceiverConfig", receiverMessage)
	}
	if receiver.StreamSettings == nil {
		t.Fatal("expected stream settings to be present")
	}
	if got := receiver.StreamSettings.SecurityType; got == "" {
		t.Fatal("expected stream security type to be populated")
	}
	if len(receiver.StreamSettings.SecuritySettings) == 0 {
		t.Fatal("expected tls security settings")
	}

	securityMessage, err := receiver.StreamSettings.SecuritySettings[0].GetInstance()
	if err != nil {
		t.Fatalf("decode tls security settings: %v", err)
	}
	tlsConfig, ok := securityMessage.(*xtlstls.Config)
	if !ok {
		t.Fatalf("tls security settings type = %T, want *tls.Config", securityMessage)
	}
	if got := tlsConfig.GetEchServerKeys(); string(got) != string(rawECHKeys) {
		t.Fatalf("EchServerKeys = %q, want %q", string(got), string(rawECHKeys))
	}
}
