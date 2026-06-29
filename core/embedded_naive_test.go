package core

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/base64"
	"io"
	"net"
	"net/http"
	"strconv"
	"testing"
	"time"

	panel "github.com/wyx2685/v2node/api/v2board"
)

func TestEmbeddedNaiveStopReleasesTCPPort(t *testing.T) {
	certFile, keyFile := writeSelfSignedCert(t)
	port := unusedTCPPortForCore(t)
	node := &panel.NodeInfo{Id: 9, Type: "naive", Common: &panel.CommonNode{Protocol: "naive", ServerPort: port, ListenIP: "127.0.0.1", TlsSettings: panel.TlsSettings{CertFile: certFile, KeyFile: keyFile}}}
	server, err := newEmbeddedNaiveServer(node, []panel.UserInfo{{Id: 1, Uuid: "11111111-1111-1111-1111-111111111111"}})
	if err != nil {
		t.Fatalf("newEmbeddedNaiveServer() error = %v", err)
	}
	if err := server.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	waitTCPPortBusy(t, port)
	if err := server.Stop(); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	waitTCPPortFree(t, port)
}

func TestEmbeddedNaiveConnectReportsTrafficAndAccess(t *testing.T) {
	echoLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen echo: %v", err)
	}
	defer echoLn.Close()
	go func() {
		conn, err := echoLn.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_, _ = io.Copy(conn, conn)
	}()

	certFile, keyFile := writeSelfSignedCert(t)
	port := unusedTCPPortForCore(t)
	node := &panel.NodeInfo{Id: 901, Type: "naive", Common: &panel.CommonNode{Protocol: "naive", ServerPort: port, ListenIP: "127.0.0.1", TlsSettings: panel.TlsSettings{CertFile: certFile, KeyFile: keyFile}, SensitiveAudit: &panel.SensitiveAuditConfig{Enable: true, Rules: []string{"domain:127.0.0.1"}, LogClientIP: true}}}
	user := panel.UserInfo{Id: 9001, Uuid: "33333333-3333-3333-3333-333333333333"}
	server, err := newEmbeddedNaiveServer(node, []panel.UserInfo{user})
	if err != nil {
		t.Fatalf("newEmbeddedNaiveServer() error = %v", err)
	}
	if err := server.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer server.Stop()
	waitTCPPortBusy(t, port)

	conn, err := tls.Dial("tcp", net.JoinHostPort("127.0.0.1", portString(port)), &tls.Config{InsecureSkipVerify: true, NextProtos: []string{"http/1.1"}})
	if err != nil {
		t.Fatalf("dial naive: %v", err)
	}
	defer conn.Close()
	auth := base64.StdEncoding.EncodeToString([]byte("9001:" + user.Uuid))
	target := echoLn.Addr().String()
	_, _ = io.WriteString(conn, "CONNECT "+target+" HTTP/1.1\r\nHost: "+target+"\r\nProxy-Authorization: Basic "+auth+"\r\n\r\n")
	resp, err := http.ReadResponse(bufio.NewReader(conn), &http.Request{Method: http.MethodConnect})
	if err != nil {
		t.Fatalf("read CONNECT response: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("CONNECT status = %d, want 200", resp.StatusCode)
	}
	payload := "hello-naive"
	if _, err := io.WriteString(conn, payload); err != nil {
		t.Fatalf("write tunnel: %v", err)
	}
	buf := make([]byte, len(payload))
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatalf("read echo: %v", err)
	}
	if string(buf) != payload {
		t.Fatalf("echo = %q, want %q", string(buf), payload)
	}
	_ = conn.Close()
	waitEmbeddedTraffic(t, node.Id, user.Uuid)

	collector := NewExternalTrafficCollector(node)
	if got, err := collector.CollectTraffic([]panel.UserInfo{user}, 0); err != nil || len(got) != 0 {
		t.Fatalf("first CollectTraffic() = %#v, err=%v; want baseline", got, err)
	}
	observer := embeddedTrafficObserver{nodeID: node.Id}
	observer.AddTraffic(user.Uuid, 5, 7, "127.0.0.1")
	traffic, err := collector.CollectTraffic([]panel.UserInfo{user}, 0)
	if err != nil {
		t.Fatalf("second CollectTraffic() error = %v", err)
	}
	if len(traffic) != 1 || traffic[0].UID != user.Id || traffic[0].Upload < 5 || traffic[0].Download < 7 {
		t.Fatalf("second CollectTraffic() = %#v, want naive embedded delta", traffic)
	}
	access, err := collector.CollectSensitiveAccess([]panel.UserInfo{user})
	if err != nil {
		t.Fatalf("CollectSensitiveAccess() error = %v", err)
	}
	if len(access) != 1 || access[0].UserID != user.Id || access[0].Domain != "127.0.0.1" || access[0].Rule != "domain:127.0.0.1" || access[0].ClientIP != "127.0.0.1" {
		t.Fatalf("CollectSensitiveAccess() = %#v, want target access", access)
	}
}

func TestEmbeddedSensitiveObserverFiltersRules(t *testing.T) {
	node := &panel.NodeInfo{Id: 902, Type: "naive", Common: &panel.CommonNode{Protocol: "naive", SensitiveAudit: &panel.SensitiveAuditConfig{Enable: true, Rules: []string{"domain:exact.example", "suffix:blocked.example", "keyword:danger"}, LogClientIP: false}}}
	user := panel.UserInfo{Id: 9002, Uuid: "44444444-4444-4444-4444-444444444444"}
	observer := newEmbeddedTrafficObserver(node)
	observer.AddAccess(user.Uuid, "safe.example", "tcp", "203.0.113.9")
	observer.AddAccess(user.Uuid, "api.blocked.example", "tcp", "203.0.113.9")
	observer.AddAccess(user.Uuid, "danger-zone.example", "tcp", "203.0.113.9")

	collector := NewExternalTrafficCollector(node)
	access, err := collector.CollectSensitiveAccess([]panel.UserInfo{user})
	if err != nil {
		t.Fatalf("CollectSensitiveAccess() error = %v", err)
	}
	if len(access) != 2 {
		t.Fatalf("CollectSensitiveAccess() len = %d, want 2: %#v", len(access), access)
	}
	byDomain := map[string]panel.SensitiveAccessEvent{}
	for _, event := range access {
		byDomain[event.Domain] = event
		if event.ClientIP != "" {
			t.Fatalf("ClientIP = %q, want hidden", event.ClientIP)
		}
	}
	if byDomain["api.blocked.example"].Rule != "suffix:blocked.example" || byDomain["danger-zone.example"].Rule != "keyword:danger" {
		t.Fatalf("events = %#v, want matched rules", access)
	}
}

func waitEmbeddedTraffic(t *testing.T, nodeID int, uuid string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		snapshot := globalEmbeddedTraffic.snapshot(nodeID, map[int]string{1: uuid})[1]
		if snapshot.Upload > 0 || snapshot.Download > 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("embedded traffic for node=%d uuid=%s was not recorded", nodeID, uuid)
}

func portString(port int) string {
	return strconv.Itoa(port)
}

func unusedTCPPortForCore(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen tcp: %v", err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}
