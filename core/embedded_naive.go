package core

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	panel "github.com/wyx2685/v2node/api/v2board"
	"golang.org/x/net/http2"
)

type embeddedNaiveServer struct {
	node     *panel.NodeInfo
	users    []panel.UserInfo
	server   *http.Server
	listener net.Listener
	mu       sync.Mutex
	cancel   context.CancelFunc
}

func newEmbeddedNaiveServer(node *panel.NodeInfo, users []panel.UserInfo) (EmbeddedProtocolServer, error) {
	return &embeddedNaiveServer{node: node, users: append([]panel.UserInfo(nil), users...)}, nil
}

func (s *embeddedNaiveServer) Protocol() string { return "naive" }

func (s *embeddedNaiveServer) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.server != nil {
		return nil
	}
	if s.node == nil || s.node.Common == nil {
		return fmt.Errorf("missing naive node config")
	}
	certFile := s.node.Common.CertInfoFile()
	keyFile := s.node.Common.CertKeyFile()
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return err
	}
	listenHost := strings.TrimSpace(s.node.Common.ListenIP)
	if listenHost == "" {
		listenHost = "0.0.0.0"
	}
	addr := net.JoinHostPort(listenHost, strconv.Itoa(s.node.Common.ServerPort))
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	childCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	s.listener = ln
	userByName, uuidByPassword := buildNaiveAuthMaps(s.users)
	observer := newEmbeddedTrafficObserver(s.node)
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		username, password, ok := naiveBasicAuth(r)
		if !ok {
			w.Header().Set("Proxy-Authenticate", `Basic realm="v2node"`)
			http.Error(w, "proxy authentication required", http.StatusProxyAuthRequired)
			return
		}
		uuid := uuidByPassword[password]
		if expected, exists := userByName[username]; exists && expected == password {
			uuid = password
		}
		if uuid == "" {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		serveNaiveConnect(w, r, uuid, observer)
	})
	httpServer := &http.Server{Handler: h, TLSConfig: &tls.Config{Certificates: []tls.Certificate{cert}, NextProtos: []string{"h2", "http/1.1"}}}
	_ = http2.ConfigureServer(httpServer, &http2.Server{})
	s.server = httpServer
	go func() {
		go func() {
			<-childCtx.Done()
			_ = httpServer.Close()
		}()
		_ = httpServer.ServeTLS(ln, "", "")
	}()
	return nil
}

func (s *embeddedNaiveServer) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		s.cancel()
	}
	s.cancel = nil
	var err error
	if s.server != nil {
		err = s.server.Close()
		s.server = nil
	}
	if s.listener != nil {
		_ = s.listener.Close()
		s.listener = nil
	}
	return err
}

func naiveBasicAuth(r *http.Request) (string, string, bool) {
	if r == nil {
		return "", "", false
	}
	if username, password, ok := r.BasicAuth(); ok {
		return username, password, true
	}
	value := strings.TrimSpace(r.Header.Get("Proxy-Authorization"))
	if value == "" {
		return "", "", false
	}
	cloned := new(http.Request)
	*cloned = *r
	cloned.Header = r.Header.Clone()
	cloned.Header.Set("Authorization", value)
	return cloned.BasicAuth()
}

func buildNaiveAuthMaps(users []panel.UserInfo) (map[string]string, map[string]string) {
	byName := make(map[string]string, len(users)*2)
	byPassword := make(map[string]string, len(users))
	for _, user := range users {
		if strings.TrimSpace(user.Uuid) == "" {
			continue
		}
		password := user.Uuid
		byName[strconv.Itoa(user.Id)] = password
		byName[user.Uuid] = password
		byPassword[password] = user.Uuid
	}
	return byName, byPassword
}

func serveNaiveConnect(w http.ResponseWriter, r *http.Request, uuid string, observer embeddedTrafficObserver) {
	target := r.Host
	if !strings.Contains(target, ":") {
		target = net.JoinHostPort(target, "443")
	}
	observer.AddAccess(uuid, hostWithoutPort(target), "tcp", remoteIP(r.RemoteAddr))
	upstream, err := net.DialTimeout("tcp", target, 10*time.Second)
	if err != nil {
		http.Error(w, "bad gateway", http.StatusBadGateway)
		return
	}
	defer upstream.Close()
	clientIP := remoteIP(r.RemoteAddr)
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		serveNaiveConnectHTTP2(w, r, upstream, uuid, clientIP, observer)
		return
	}
	w.WriteHeader(http.StatusOK)
	clientConn, _, err := hijacker.Hijack()
	if err != nil {
		return
	}
	defer clientConn.Close()
	done := make(chan struct{}, 2)
	go func() {
		n, _ := io.Copy(upstream, clientConn)
		observer.AddTraffic(uuid, n, 0, clientIP)
		done <- struct{}{}
	}()
	go func() {
		n, _ := io.Copy(clientConn, upstream)
		observer.AddTraffic(uuid, 0, n, clientIP)
		done <- struct{}{}
	}()
	<-done
}

func serveNaiveConnectHTTP2(w http.ResponseWriter, r *http.Request, upstream net.Conn, uuid string, clientIP string, observer embeddedTrafficObserver) {
	if controller := http.NewResponseController(w); controller != nil {
		_ = controller.EnableFullDuplex()
	}
	w.WriteHeader(http.StatusOK)
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
	done := make(chan struct{}, 2)
	go func() {
		n, _ := io.Copy(upstream, r.Body)
		observer.AddTraffic(uuid, n, 0, clientIP)
		_ = upstream.Close()
		done <- struct{}{}
	}()
	go func() {
		n, _ := io.Copy(flushWriter{ResponseWriter: w}, upstream)
		observer.AddTraffic(uuid, 0, n, clientIP)
		done <- struct{}{}
	}()
	<-done
}

func hostWithoutPort(addr string) string {
	host, _, err := net.SplitHostPort(addr)
	if err == nil {
		return host
	}
	return addr
}

func remoteIP(addr string) string {
	host, _, err := net.SplitHostPort(addr)
	if err == nil {
		return host
	}
	return addr
}

type flushWriter struct{ http.ResponseWriter }

func (w flushWriter) Write(p []byte) (int, error) {
	n, err := w.ResponseWriter.Write(p)
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
	return n, err
}
