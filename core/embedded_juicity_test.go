package core

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

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

func TestEmbeddedJuicityStopReleasesUDPPort(t *testing.T) {
	certFile, keyFile := writeSelfSignedCert(t)
	port := unusedUDPPort(t)
	node := &panel.NodeInfo{Id: 7, Type: "juicity", Common: &panel.CommonNode{Protocol: "juicity", ServerPort: port, ListenIP: "127.0.0.1", TlsSettings: panel.TlsSettings{CertFile: certFile, KeyFile: keyFile}}}
	server, err := newEmbeddedJuicityServer(node, []panel.UserInfo{{Id: 1, Uuid: "11111111-1111-1111-1111-111111111111"}})
	if err != nil {
		t.Fatalf("newEmbeddedJuicityServer() error = %v", err)
	}
	if err := server.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	waitUDPPortBusy(t, port)
	if err := server.Stop(); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	waitUDPPortFree(t, port)
}

func writeSelfSignedCert(t *testing.T) (string, string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "localhost"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), DNSNames: []string{"localhost"}}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("CreateCertificate: %v", err)
	}
	dir := t.TempDir()
	certFile := filepath.Join(dir, "cert.pem")
	keyFile := filepath.Join(dir, "key.pem")
	certOut, err := os.Create(certFile)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}
	if err := pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: der}); err != nil {
		t.Fatalf("write cert: %v", err)
	}
	_ = certOut.Close()
	keyOut, err := os.Create(keyFile)
	if err != nil {
		t.Fatalf("create key: %v", err)
	}
	if err := pem.Encode(keyOut, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}); err != nil {
		t.Fatalf("write key: %v", err)
	}
	_ = keyOut.Close()
	return certFile, keyFile
}

func unusedUDPPort(t *testing.T) int {
	t.Helper()
	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen udp: %v", err)
	}
	defer conn.Close()
	return conn.LocalAddr().(*net.UDPAddr).Port
}

func waitUDPPortBusy(t *testing.T, port int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.ListenPacket("udp", net.JoinHostPort("127.0.0.1", fmt.Sprint(port)))
		if err != nil {
			return
		}
		_ = conn.Close()
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("udp port %d did not become busy", port)
}

func waitUDPPortFree(t *testing.T, port int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.ListenPacket("udp", net.JoinHostPort("127.0.0.1", fmt.Sprint(port)))
		if err == nil {
			_ = conn.Close()
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("udp port %d was not released", port)
}

func TestJuicityEmbeddedTrafficRegistryFeedsCollector(t *testing.T) {
	node := &panel.NodeInfo{Id: 707, Type: "juicity", Common: &panel.CommonNode{Protocol: "juicity"}}
	collector := NewExternalTrafficCollector(node)
	users := []panel.UserInfo{{Id: 7001, Uuid: "11111111-1111-1111-1111-111111111111"}}
	observer := embeddedTrafficObserver{nodeID: node.Id}

	observer.AddTraffic(users[0].Uuid, 1000, 2000, "203.0.113.7")
	got, err := collector.CollectTraffic(users, 0)
	if err != nil {
		t.Fatalf("first CollectTraffic() error = %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("first CollectTraffic() = %#v, want baseline", got)
	}

	observer.AddTraffic(users[0].Uuid, 300, 400, "203.0.113.7")
	got, err = collector.CollectTraffic(users, 0)
	if err != nil {
		t.Fatalf("second CollectTraffic() error = %v", err)
	}
	if len(got) != 1 || got[0].UID != 7001 || got[0].Upload != 300 || got[0].Download != 400 {
		t.Fatalf("second CollectTraffic() = %#v, want uid=7001 up=300 down=400", got)
	}
}
