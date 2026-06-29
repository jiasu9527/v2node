package core

import (
	"context"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	mieruServer "github.com/enfein/mieru/v3/apis/server"
	"github.com/enfein/mieru/v3/pkg/appctl/appctlpb"
	panel "github.com/wyx2685/v2node/api/v2board"
	"google.golang.org/protobuf/proto"
)

type embeddedMieruServer struct {
	node   *panel.NodeInfo
	users  []panel.UserInfo
	server mieruServer.Server
	mu     sync.Mutex
	cancel context.CancelFunc
}

func newEmbeddedMieruServer(node *panel.NodeInfo, users []panel.UserInfo) (EmbeddedProtocolServer, error) {
	return &embeddedMieruServer{node: node, users: append([]panel.UserInfo(nil), users...)}, nil
}

func (s *embeddedMieruServer) Protocol() string { return "mieru" }

func (s *embeddedMieruServer) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.server != nil && s.server.IsRunning() {
		return nil
	}
	cfg, err := buildMieruServerConfig(s.node, s.users)
	if err != nil {
		return err
	}
	server := mieruServer.NewServer()
	if err := server.Store(&mieruServer.ServerConfig{Config: cfg}); err != nil {
		return err
	}
	if err := server.Start(); err != nil {
		return err
	}
	childCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	s.server = server
	go s.acceptLoop(childCtx, server)
	return nil
}

func (s *embeddedMieruServer) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		s.cancel()
	}
	s.cancel = nil
	if s.server != nil {
		err := s.server.Stop()
		s.server = nil
		return err
	}
	return nil
}

func (s *embeddedMieruServer) acceptLoop(ctx context.Context, server mieruServer.Server) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		conn, req, err := server.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return
			default:
			}
			time.Sleep(100 * time.Millisecond)
			continue
		}
		go relayMieruAcceptedConn(conn, req.DstAddr.String())
	}
}

func relayMieruAcceptedConn(client net.Conn, target string) {
	defer client.Close()
	upstream, err := net.DialTimeout("tcp", target, 10*time.Second)
	if err != nil {
		return
	}
	defer upstream.Close()
	done := make(chan struct{}, 2)
	go func() { _, _ = io.Copy(upstream, client); done <- struct{}{} }()
	go func() { _, _ = io.Copy(client, upstream); done <- struct{}{} }()
	<-done
}

func buildMieruServerConfig(node *panel.NodeInfo, users []panel.UserInfo) (*appctlpb.ServerConfig, error) {
	if node == nil || node.Common == nil {
		return nil, fmt.Errorf("missing mieru node config")
	}
	transport := appctlpb.TransportProtocol_TCP
	if strings.EqualFold(strings.TrimSpace(node.Common.Transport), "UDP") {
		transport = appctlpb.TransportProtocol_UDP
	}
	port := int32(node.Common.ServerPort)
	if port < 0 || port > 65535 {
		return nil, fmt.Errorf("invalid mieru port: %d", node.Common.ServerPort)
	}
	mtu := int32(node.Common.MTU)
	if mtu <= 0 {
		mtu = 1400
	}
	pbUsers := make([]*appctlpb.User, 0, len(users))
	for _, user := range users {
		if strings.TrimSpace(user.Uuid) == "" {
			continue
		}
		pbUsers = append(pbUsers, &appctlpb.User{Name: proto.String(strconv.Itoa(user.Id)), Password: proto.String(user.Uuid)})
	}
	if len(pbUsers) == 0 {
		return nil, fmt.Errorf("missing mieru users")
	}
	return &appctlpb.ServerConfig{
		PortBindings: []*appctlpb.PortBinding{{Port: proto.Int32(port), Protocol: &transport}},
		Users:        pbUsers,
		Mtu:          proto.Int32(mtu),
		LoggingLevel: appctlpb.LoggingLevel_WARN.Enum(),
		AdvancedSettings: &appctlpb.ServerAdvancedSettings{
			UserHintIsMandatory: proto.Bool(false),
		},
	}, nil
}
