package core

import (
	"context"
	"fmt"
	"strings"
	"sync"

	panel "github.com/wyx2685/v2node/api/v2board"
)

// EmbeddedProtocolServer is a protocol server owned by the v2node process.
// It intentionally does not shell out to juicity-server / mita, so lifecycle,
// reload and statistics can be controlled the same way as built-in protocols.
type EmbeddedProtocolServer interface {
	Protocol() string
	Start(context.Context) error
	Stop() error
}

type embeddedProtocolServer struct {
	protocol string
	node     *panel.NodeInfo
	users    []panel.UserInfo
	mu       sync.Mutex
	cancel   context.CancelFunc
	started  bool
}

func NewEmbeddedProtocolServer(node *panel.NodeInfo, users []panel.UserInfo) (EmbeddedProtocolServer, error) {
	if node == nil || node.Common == nil {
		return nil, fmt.Errorf("missing embedded protocol node config")
	}
	protocol := strings.ToLower(strings.TrimSpace(node.Type))
	if protocol == "" {
		protocol = strings.ToLower(strings.TrimSpace(node.Common.Protocol))
	}
	switch protocol {
	case "mieru":
		return newEmbeddedMieruServer(node, users)
	case "juicity":
		return newEmbeddedJuicityServer(node, users)
	case "naive":
		return newEmbeddedNaiveServer(node, users)
	default:
		return nil, fmt.Errorf("unsupported embedded protocol: %s", protocol)
	}
}

func (s *embeddedProtocolServer) Protocol() string {
	if s == nil {
		return ""
	}
	return s.protocol
}

func (s *embeddedProtocolServer) Start(ctx context.Context) error {
	if s == nil {
		return fmt.Errorf("nil embedded protocol server")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.started {
		return nil
	}
	childCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	s.started = true
	go func() {
		<-childCtx.Done()
	}()
	return nil
}

func (s *embeddedProtocolServer) Stop() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		s.cancel()
	}
	s.cancel = nil
	s.started = false
	return nil
}
