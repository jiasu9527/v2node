package anytls

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	M "github.com/sagernet/sing/common/metadata"
	"github.com/sagernet/sing/common/uot"
	"github.com/xtls/xray-core/common/buf"
	xnet "github.com/xtls/xray-core/common/net"
	"github.com/xtls/xray-core/common/protocol"
	"github.com/xtls/xray-core/common/serial"
	sessionctx "github.com/xtls/xray-core/common/session"
	"github.com/xtls/xray-core/core"
	"github.com/xtls/xray-core/features/policy"
	"github.com/xtls/xray-core/features/routing"
	"github.com/xtls/xray-core/transport"
	"github.com/xtls/xray-core/transport/pipe"
)

const testPassword = "local-lifecycle-test"

type testPolicy struct{}

func (*testPolicy) Type() interface{} { return policy.ManagerType() }
func (*testPolicy) Start() error      { return nil }
func (*testPolicy) Close() error      { return nil }
func (*testPolicy) ForLevel(uint32) policy.Session {
	return policy.Session{Timeouts: policy.Timeout{Handshake: 3 * time.Second}}
}
func (*testPolicy) ForSystem() policy.System { return policy.System{} }

func testServer(t *testing.T) *Server {
	t.Helper()
	instance := new(core.Instance)
	if err := instance.AddFeature(&testPolicy{}); err != nil {
		t.Fatal(err)
	}
	ctx := context.WithValue(context.Background(), core.XrayKey(1), instance)
	s, err := NewServer(ctx, &ServerConfig{Users: []*protocol.User{{Email: "test", Account: serial.ToTypedMessage(&Account{Password: testPassword})}}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

type testDispatcher struct {
	dispatch func(context.Context, xnet.Destination) (*transport.Link, error)
}

func (*testDispatcher) Type() interface{} { return routing.DispatcherType() }
func (*testDispatcher) Start() error      { return nil }
func (*testDispatcher) Close() error      { return nil }
func (d *testDispatcher) Dispatch(c context.Context, dest xnet.Destination) (*transport.Link, error) {
	return d.dispatch(c, dest)
}
func (*testDispatcher) DispatchLink(context.Context, xnet.Destination, *transport.Link) error {
	return nil
}

type testPipes struct {
	upR, downR *pipe.Reader
	upW, downW *pipe.Writer
}

func newTestPipes(t *testing.T) *testPipes {
	t.Helper()
	ur, uw := pipe.New(pipe.WithSizeLimit(0))
	dr, dw := pipe.New(pipe.WithSizeLimit(0))
	p := &testPipes{ur, dr, uw, dw}
	t.Cleanup(func() { ur.Interrupt(); uw.Interrupt(); dr.Interrupt(); dw.Interrupt() })
	return p
}
func (p *testPipes) link() *transport.Link { return &transport.Link{Reader: p.downR, Writer: p.upW} }
func assertReaderClosed(t *testing.T, r *pipe.Reader) {
	t.Helper()
	mb, err := r.ReadMultiBufferTimeout(time.Second)
	buf.ReleaseMulti(mb)
	if err == nil || err == buf.ErrReadTimeout {
		t.Fatalf("reader still open: %v", err)
	}
}
func waitDone(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(3 * time.Second):
		t.Fatal("cleanup did not finish")
	}
}

type wireClient struct {
	c      net.Conn
	cancel context.CancelFunc
	done   chan struct{}
}

func newWireClient(t *testing.T, s *Server, d routing.Dispatcher) *wireClient {
	t.Helper()
	sc, cc := net.Pipe()
	cc.SetDeadline(time.Now().Add(5 * time.Second))
	ctx, cancel := context.WithCancel(sessionctx.ContextWithInbound(context.Background(), &sessionctx.Inbound{}))
	w := &wireClient{cc, cancel, make(chan struct{})}
	go func() { s.Process(ctx, xnet.Network_TCP, sc, d); close(w.done) }()
	t.Cleanup(func() { cc.Close(); cancel(); waitDone(t, w.done) })
	sum := sha256.Sum256([]byte(testPassword))
	auth := append(sum[:], 0, 0)
	if _, err := cc.Write(auth); err != nil {
		t.Fatal(err)
	}
	w.send(t, cmdSettings, 0, []byte("v=2"))
	w.receive(t, cmdServerSettings, 0)
	return w
}
func (w *wireClient) send(t *testing.T, cmd byte, sid uint32, payload []byte) {
	t.Helper()
	h := make([]byte, 7)
	h[0] = cmd
	binary.BigEndian.PutUint32(h[1:5], sid)
	binary.BigEndian.PutUint16(h[5:7], uint16(len(payload)))
	if _, err := w.c.Write(append(h, payload...)); err != nil {
		t.Fatal(err)
	}
}
func (w *wireClient) receive(t *testing.T, cmd byte, sid uint32) []byte {
	t.Helper()
	var h [7]byte
	if _, err := io.ReadFull(w.c, h[:]); err != nil {
		t.Fatal(err)
	}
	if h[0] != cmd || binary.BigEndian.Uint32(h[1:5]) != sid {
		t.Fatalf("unexpected frame %v", h)
	}
	b := make([]byte, binary.BigEndian.Uint16(h[5:7]))
	if _, err := io.ReadFull(w.c, b); err != nil {
		t.Fatal(err)
	}
	return b
}
func streamPayload(t *testing.T, udp bool) []byte {
	t.Helper()
	b := buf.New()
	defer b.Release()
	dest := M.ParseSocksaddr("127.0.0.1:80")
	if udp {
		dest = M.Socksaddr{Fqdn: "sp.v2.udp-over-tcp.arpa"}
	}
	if err := M.SocksaddrSerializer.WriteAddrPort(b, dest); err != nil {
		t.Fatal(err)
	}
	if udp {
		if err := uot.WriteRequest(b, uot.Request{IsConnect: true, Destination: M.ParseSocksaddr("127.0.0.1:53")}); err != nil {
			t.Fatal(err)
		}
	}
	return append([]byte(nil), b.Bytes()...)
}

func TestServerStreamLifecycle(t *testing.T) {
	for _, udp := range []bool{false, true} {
		for _, end := range []string{"disconnect", "fin", "cancel", "server-close"} {
			name := "tcp/" + end
			if udp {
				name = "udp/" + end
			}
			t.Run(name, func(t *testing.T) {
				s := testServer(t)
				p := newTestPipes(t)
				dispatched := make(chan struct{})
				d := &testDispatcher{dispatch: func(_ context.Context, dest xnet.Destination) (*transport.Link, error) {
					want := xnet.Network_TCP
					if udp {
						want = xnet.Network_UDP
					}
					if dest.Network != want {
						t.Errorf("network %v != %v", dest.Network, want)
					}
					close(dispatched)
					return p.link(), nil
				}}
				w := newWireClient(t, s, d)
				w.send(t, cmdSYN, 1, nil)
				w.send(t, cmdPSH, 1, streamPayload(t, udp))
				w.receive(t, cmdSYNACK, 1)
				waitDone(t, dispatched)
				// Exercise both directions before closing; UoT framing is handled by v2node.
				w.send(t, cmdPSH, 1, []byte("request"))
				mb, err := p.upR.ReadMultiBufferTimeout(time.Second)
				if err != nil {
					t.Fatal(err)
				}
				got := make([]byte, mb.Len())
				mb.Copy(got)
				buf.ReleaseMulti(mb)
				if string(got) != "request" {
					t.Fatalf("uplink %q", got)
				}
				if err := p.downW.WriteMultiBuffer(buf.MergeBytes(nil, []byte("response"))); err != nil {
					t.Fatal(err)
				}
				if string(w.receive(t, cmdPSH, 1)) != "response" {
					t.Fatal("downlink data mismatch")
				}
				switch end {
				case "disconnect":
					w.c.Close()
					waitDone(t, w.done)
				case "fin":
					w.send(t, cmdFIN, 1, nil)
					w.receive(t, cmdFIN, 1)
				case "cancel":
					w.cancel()
					waitDone(t, w.done)
				case "server-close":
					done := make(chan struct{})
					go func() { s.Close(); close(done) }()
					waitDone(t, done)
					waitDone(t, w.done)
				}
				assertReaderClosed(t, p.upR)
				assertReaderClosed(t, p.downR)
			})
		}
	}
}

func TestServerCloseStopsUpdaterAndRejectsUsers(t *testing.T) {
	s := testServer(t)
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.AddUser(context.Background(), &protocol.MemoryUser{Email: "test", Account: &MemoryAccount{Password: testPassword}})
			s.RemoveUser(context.Background(), "test")
			s.Close()
		}()
	}
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	waitDone(t, done)
	waitDone(t, s.updaterDone)
	if err := s.AddUser(context.Background(), &protocol.MemoryUser{Email: "test", Account: &MemoryAccount{Password: testPassword}}); err == nil {
		t.Fatal("AddUser accepted after Close")
	}
	if s.GetUsersCount(context.Background()) != 0 {
		t.Fatal("user snapshot retained")
	}
}

func TestServerCloseDuringHandshake(t *testing.T) {
	s := testServer(t)
	sc, cc := net.Pipe()
	defer cc.Close()
	done := make(chan struct{})
	ctx := sessionctx.ContextWithInbound(context.Background(), &sessionctx.Inbound{})
	go func() { s.Process(ctx, xnet.Network_TCP, sc, nil); close(done) }()
	// Unblocks once Process has registered the session and started auth reading.
	cc.SetWriteDeadline(time.Now().Add(time.Second))
	if _, err := cc.Write([]byte{1}); err != nil {
		t.Fatal(err)
	}
	s.Close()
	waitDone(t, done)
}

func TestServerClosesLateDispatchedLink(t *testing.T) {
	s := testServer(t)
	p := newTestPipes(t)
	entered := make(chan struct{})
	d := &testDispatcher{dispatch: func(ctx context.Context, _ xnet.Destination) (*transport.Link, error) {
		close(entered)
		<-ctx.Done()
		return p.link(), nil
	}}
	w := newWireClient(t, s, d)
	w.send(t, cmdSYN, 1, nil)
	w.send(t, cmdPSH, 1, streamPayload(t, false))
	waitDone(t, entered)
	w.cancel()
	waitDone(t, w.done)
	assertReaderClosed(t, p.upR)
	assertReaderClosed(t, p.downR)
}

func TestStreamPublishRacesWithClose(t *testing.T) {
	for i := 0; i < 64; i++ {
		p := newTestPipes(t)
		st := newStream(1, nil)
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); st.setLink(p.link()) }()
		go func() { defer wg.Done(); st.abort(io.EOF) }()
		wg.Wait()
		assertReaderClosed(t, p.upR)
		assertReaderClosed(t, p.downR)
	}
}

func TestServerDispatchFailureFinishesStream(t *testing.T) {
	s := testServer(t)
	d := &testDispatcher{dispatch: func(context.Context, xnet.Destination) (*transport.Link, error) { return nil, io.ErrClosedPipe }}
	w := newWireClient(t, s, d)
	for i := uint32(1); i <= 16; i++ {
		w.send(t, cmdSYN, i, nil)
		w.send(t, cmdPSH, i, append(streamPayload(t, false), []byte("data")...))
		w.receive(t, cmdFIN, i)
	}
	s.sessionsMu.Lock()
	defer s.sessionsMu.Unlock()
	for sess := range s.sessions {
		sess.streamsMu.Lock()
		n := len(sess.streams)
		sess.streamsMu.Unlock()
		if n != 0 {
			t.Fatalf("failed streams retained: %d", n)
		}
	}
}

func TestStreamCloseDrainsUplink(t *testing.T) {
	p := newTestPipes(t)
	st := newStream(1, p.link())
	if err := p.upW.WriteMultiBuffer(buf.MergeBytes(nil, []byte("last request"))); err != nil {
		t.Fatal(err)
	}
	st.close(nil)
	mb, err := p.upR.ReadMultiBufferTimeout(time.Second)
	if err != nil {
		t.Fatal(err)
	}
	buf.ReleaseMulti(mb)
	assertReaderClosed(t, p.upR)
	assertReaderClosed(t, p.downR)
}

func TestServerCancelUnblocksBackpressuredUplink(t *testing.T) {
	s := testServer(t)
	p := newTestPipes(t)
	entered := make(chan struct{})
	d := &testDispatcher{dispatch: func(context.Context, xnet.Destination) (*transport.Link, error) { close(entered); return p.link(), nil }}
	w := newWireClient(t, s, d)
	w.send(t, cmdSYN, 1, nil)
	w.send(t, cmdPSH, 1, streamPayload(t, false))
	w.receive(t, cmdSYNACK, 1)
	waitDone(t, entered)
	w.send(t, cmdPSH, 1, []byte("fills pipe"))
	w.send(t, cmdPSH, 1, []byte("blocks until cancel"))
	w.cancel()
	waitDone(t, w.done)
	assertReaderClosed(t, p.downR)
	// An abort discards buffered uplink data rather than retaining it forever.
	assertReaderClosed(t, p.upR)
}

func TestServerRepeatedConnectDisconnectReleasesSessions(t *testing.T) {
	s := testServer(t)
	for i := 0; i < 30; i++ {
		p := newTestPipes(t)
		d := &testDispatcher{dispatch: func(context.Context, xnet.Destination) (*transport.Link, error) { return p.link(), nil }}
		w := newWireClient(t, s, d)
		w.send(t, cmdSYN, 1, nil)
		w.send(t, cmdPSH, 1, streamPayload(t, false))
		w.receive(t, cmdSYNACK, 1)
		w.c.Close()
		waitDone(t, w.done)
		assertReaderClosed(t, p.upR)
		assertReaderClosed(t, p.downR)
		s.sessionsMu.Lock()
		n := len(s.sessions)
		s.sessionsMu.Unlock()
		if n != 0 {
			t.Fatalf("cycle %d retained %d sessions", i, n)
		}
	}
}
