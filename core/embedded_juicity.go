package core

import (
	"context"
	"fmt"
	"strings"
	"sync"

	juicityLog "github.com/juicity/juicity/pkg/log"
	panel "github.com/wyx2685/v2node/api/v2board"
	juicityServer "github.com/wyx2685/v2node/core/juicityembedded"
)

type embeddedJuicityServer struct {
	node   *panel.NodeInfo
	users  []panel.UserInfo
	mu     sync.Mutex
	cancel context.CancelFunc
	server *juicityServer.Server
}

func newEmbeddedJuicityServer(node *panel.NodeInfo, users []panel.UserInfo) (EmbeddedProtocolServer, error) {
	return &embeddedJuicityServer{node: node, users: append([]panel.UserInfo(nil), users...)}, nil
}

func (s *embeddedJuicityServer) Protocol() string { return "juicity" }

func (s *embeddedJuicityServer) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.server != nil {
		return nil
	}
	opts, listen, err := buildJuicityServerOptions(s.node, s.users)
	if err != nil {
		return err
	}
	server, err := juicityServer.New(opts)
	if err != nil {
		return err
	}
	childCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	s.server = server
	go func() {
		err := server.Serve(listen)
		select {
		case <-childCtx.Done():
			return
		default:
			if err != nil {
				// juicity upstream server has no structured stop hook; leave error to caller logs later.
			}
		}
	}()
	return nil
}

func (s *embeddedJuicityServer) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		s.cancel()
	}
	s.cancel = nil
	if s.server != nil {
		err := s.server.Close()
		s.server = nil
		return err
	}
	return nil
}

func buildJuicityServerOptions(node *panel.NodeInfo, users []panel.UserInfo) (*juicityServer.Options, string, error) {
	if node == nil || node.Common == nil {
		return nil, "", fmt.Errorf("missing juicity node config")
	}
	userMap := make(map[string]string, len(users))
	for _, user := range users {
		if strings.TrimSpace(user.Uuid) == "" {
			continue
		}
		userMap[user.Uuid] = user.Uuid
	}
	if len(userMap) == 0 {
		return nil, "", fmt.Errorf("missing juicity users")
	}
	fwmark := 0
	listenHost := strings.TrimSpace(node.Common.ListenIP)
	if listenHost == "" {
		listenHost = "0.0.0.0"
	}
	listen := fmt.Sprintf("%s:%d", listenHost, node.Common.ServerPort)
	return &juicityServer.Options{
		Logger:                juicityLog.NewLogger(&juicityLog.Options{Output: ""}),
		Users:                 userMap,
		Certificate:           node.Common.CertInfoFile(),
		PrivateKey:            node.Common.CertKeyFile(),
		CongestionControl:     firstNonEmpty(node.Common.CongestionControl, "bbr"),
		Fwmark:                fwmark,
		SendThrough:           node.Common.SendThrough,
		DisableOutboundUdp443: false,
		Observer:              newEmbeddedTrafficObserver(node),
	}, listen, nil
}
