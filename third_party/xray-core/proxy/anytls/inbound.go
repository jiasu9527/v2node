package anytls

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/xtls/xray-core/common/buf"
	"github.com/xtls/xray-core/common/errors"
	xnet "github.com/xtls/xray-core/common/net"
	"github.com/xtls/xray-core/common/protocol"
	sessionctx "github.com/xtls/xray-core/common/session"
	"github.com/xtls/xray-core/core"
	"github.com/xtls/xray-core/features/policy"
	"github.com/xtls/xray-core/features/routing"
	"github.com/xtls/xray-core/transport/internet/stat"
)

type Server struct {
	policyManager policy.Manager
	// lock-free read via atomic snapshot; writers use wmu and COW
	store atomic.Value // *userStoreSnapshot
	wmu   sync.Mutex
	// coalesced updates
	pendingAdds    map[string]*protocol.MemoryUser
	pendingRemoves map[string]struct{}
	pendingMu      sync.Mutex
	updateCh       chan struct{}
	stopCh         chan struct{}
	debounce       time.Duration
	paddingScheme  string

	closed      atomic.Bool
	closeOnce   sync.Once
	sessionsMu  sync.Mutex
	sessions    map[*session]struct{}
	sessionsWG  sync.WaitGroup
	updaterDone chan struct{}
}

func NewServer(ctx context.Context, config *ServerConfig) (*Server, error) {
	v := core.MustFromContext(ctx)
	s := &Server{
		policyManager:  v.GetFeature(policy.ManagerType()).(policy.Manager),
		updateCh:       make(chan struct{}, 1),
		stopCh:         make(chan struct{}),
		updaterDone:    make(chan struct{}),
		sessions:       make(map[*session]struct{}),
		debounce:       200 * time.Millisecond,
		pendingAdds:    make(map[string]*protocol.MemoryUser),
		pendingRemoves: make(map[string]struct{}),
		paddingScheme:  config.PaddingScheme,
	}
	users := make(map[[32]byte]*protocol.MemoryUser)
	emailIndex := make(map[string][32]byte)
	for _, u := range config.Users {
		mu, err := u.ToMemoryUser()
		if err != nil {
			return nil, errors.New("anytls: bad user").Base(err)
		}
		acc, ok := mu.Account.(*MemoryAccount)
		if !ok {
			return nil, errors.New("anytls: user account type")
		}
		sum := sha256.Sum256([]byte(acc.Password))
		users[sum] = mu
		emailIndex[mu.Email] = sum
	}
	s.store.Store(&userStoreSnapshot{users: users, emailIndex: emailIndex})
	// start updater loop
	go s.userUpdaterLoop()
	return s, nil
}

func (s *Server) Network() []xnet.Network {
	return []xnet.Network{xnet.Network_TCP, xnet.Network_UNIX}
}

func (s *Server) Process(ctx context.Context, network xnet.Network, conn stat.Connection, dispatcher routing.Dispatcher) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	sessPol := s.policyManager.ForLevel(0)
	_ = conn.SetReadDeadline(time.Now().Add(sessPol.Timeouts.Handshake))

	sess := &session{
		isClient:   false,
		server:     s,
		conn:       conn,
		br:         &buf.BufferedReader{Reader: buf.NewReader(conn)},
		bw:         buf.NewBufferedWriter(buf.NewWriter(conn)),
		streams:    make(map[uint32]*stream),
		dispatcher: dispatcher,
		cancel:     cancel,
	}
	sess.fw = newFrameWriter(sess.bw)
	sess.peerVersion = 1
	s.sessionsMu.Lock()
	if s.closed.Load() {
		s.sessionsMu.Unlock()
		sess.close(io.ErrClosedPipe)
		return io.ErrClosedPipe
	}
	s.sessions[sess] = struct{}{}
	s.sessionsWG.Add(1)
	s.sessionsMu.Unlock()
	stop := context.AfterFunc(ctx, func() { sess.close(ctx.Err()) })
	defer func() {
		stop()
		sess.close(io.EOF)
		// readLoop is finished: it can no longer start another downlink pump.
		sess.downlinkWG.Wait()
		s.sessionsMu.Lock()
		delete(s.sessions, sess)
		s.sessionsMu.Unlock()
		s.sessionsWG.Done()
	}()

	// auth header: 32B sha256(password) + 2B padlen
	var h [34]byte
	if _, err := io.ReadFull(sess.br, h[:]); err != nil {
		return errors.New("anytls: read auth").Base(err)
	}

	var sum [32]byte
	copy(sum[:], h[:32])
	snap := s.loadStore()
	user := snap.users[sum]
	if user == nil {
		return errors.New("anytls: invalid user")
	}

	padlen := binary.BigEndian.Uint16(h[32:34])
	if padlen > 0 {
		if err := discardBytes(sess.br, int(padlen)); err != nil {
			return errors.New("anytls: read padding0").Base(err)
		}
	}
	_ = conn.SetReadDeadline(time.Time{})

	inb := sessionctx.InboundFromContext(ctx)
	inb.Name = protocolName
	inb.User = user
	inb.CanSpliceCopy = 3

	return sess.readLoop(ctx)
}

// Close implements common.Closable for inbound removal and core reload.
func (s *Server) Close() error {
	s.closeOnce.Do(func() {
		s.sessionsMu.Lock()
		s.closed.Store(true)
		sessions := make([]*session, 0, len(s.sessions))
		for sess := range s.sessions {
			sessions = append(sessions, sess)
		}
		s.sessionsMu.Unlock()
		close(s.stopCh)
		for _, sess := range sessions {
			sess.close(io.ErrClosedPipe)
		}
		s.sessionsWG.Wait()
		<-s.updaterDone
		s.pendingMu.Lock()
		s.pendingAdds = nil
		s.pendingRemoves = nil
		s.pendingMu.Unlock()
		s.store.Store(&userStoreSnapshot{})
	})
	return nil
}
