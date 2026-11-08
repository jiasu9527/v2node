package dispatcher

import (
	"context"
	"fmt"
	"maps"
	"sync"
	"testing"
	"time"

	"github.com/xtls/xray-core/common"
	"github.com/xtls/xray-core/common/buf"
	"github.com/xtls/xray-core/common/net"
	"github.com/xtls/xray-core/common/session"
	xcore "github.com/xtls/xray-core/core"
	"github.com/xtls/xray-core/features/outbound"
	"github.com/xtls/xray-core/transport"
	"github.com/xtls/xray-core/transport/pipe"
)

// Exercise the actual Dispatch/DispatchLink -> HTTP sniffer -> routedDispatch
// path without opening sockets. AnyTLS reuses one inbound Content across streams.
type contentTestHandler struct {
	outbound.Handler
	tag     string
	inspect func(context.Context, *transport.Link)
}

func (h *contentTestHandler) Tag() string { return h.tag }
func (h *contentTestHandler) Dispatch(ctx context.Context, link *transport.Link) {
	h.inspect(ctx, link)
	common.Close(link.Writer)
}

type contentTestManager struct {
	outbound.Manager
	handler *contentTestHandler
}

func (m *contentTestManager) GetDefaultHandler() outbound.Handler { return m.handler }
func (m *contentTestManager) GetHandler(tag string) outbound.Handler {
	if tag == m.handler.tag {
		return m.handler
	}
	return nil
}

func contentTestContext(content *session.Content) context.Context {
	ctx := context.WithValue(context.Background(), xcore.XrayKey(1), new(xcore.Instance))
	return session.ContextWithContent(ctx, content)
}

func runContentTestRequest(ctx context.Context, d *DefaultDispatcher, method string, payload string) error {
	dest := net.TCPDestination(net.ParseAddress("192.0.2.1"), 80)
	if method == "Dispatch" {
		link, err := d.Dispatch(ctx, dest)
		if err != nil {
			return err
		}
		defer common.Close(link.Writer)
		defer common.Interrupt(link.Reader)
		return link.Writer.WriteMultiBuffer(multiBufferFromBytes([]byte(payload)))
	}
	r, w := pipe.New()
	downReader, downWriter := pipe.New()
	defer common.Interrupt(r)
	defer common.Close(w)
	defer common.Interrupt(downReader)
	defer common.Close(downWriter)
	written := make(chan error, 1)
	go func() {
		written <- w.WriteMultiBuffer(multiBufferFromBytes([]byte(payload)))
		common.Close(w)
	}()
	err := d.DispatchLink(ctx, dest, &transport.Link{Reader: r, Writer: downWriter})
	if err != nil {
		return err
	}
	return <-written
}

func receiveContentTestResult(t *testing.T, result <-chan context.Context) context.Context {
	t.Helper()
	select {
	case ctx := <-result:
		return ctx
	case <-time.After(5 * time.Second):
		t.Fatal("dispatch did not reach outbound handler")
		return nil
	}
}

func TestDispatchContentIsolation(t *testing.T) {
	for _, method := range []string{"Dispatch", "DispatchLink"} {
		for _, emptyMap := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/emptyMap=%v", method, emptyMap), func(t *testing.T) {
				parent := &session.Content{SniffingRequest: session.SniffingRequest{
					Enabled: true, OverrideDestinationForProtocol: []string{"http"},
				}, SkipDNSResolve: true}
				if emptyMap {
					parent.Attributes = make(map[string]string)
				}
				ctx := contentTestContext(parent)
				results := make(chan context.Context, 2)
				h := &contentTestHandler{tag: "direct", inspect: func(ctx context.Context, link *transport.Link) {
					// The sniffed bytes must still reach the outbound unchanged.
					mb, err := link.Reader.ReadMultiBuffer()
					if err != nil {
						t.Errorf("read sniffed bytes: %v", err)
					}
					payload := make([]byte, mb.Len())
					mb.Copy(payload)
					want := "GET /test HTTP/1.1\r\nHost: " + session.ContentFromContext(ctx).Attribute("host") + "\r\n\r\n"
					if string(payload) != want {
						t.Errorf("sniffer changed request payload: %q, want %q", payload, want)
					}
					buf.ReleaseMulti(mb)
					results <- ctx
				}}
				d := &DefaultDispatcher{ohm: &contentTestManager{handler: h}}
				var previous *session.Content
				for i := 0; i < 2; i++ {
					host := fmt.Sprintf("stream%d.example", i)
					payload := "GET /test HTTP/1.1\r\nHost: " + host + "\r\n\r\n"
					if err := runContentTestRequest(ctx, d, method, payload); err != nil {
						t.Fatal(err)
					}
					outCtx := receiveContentTestResult(t, results)
					got := session.ContentFromContext(outCtx)
					if got == parent || got == previous {
						t.Fatal("dispatch reused another stream's Content")
					}
					if got.Attribute("host") != host || got.Attribute(":path") != "/test" || got.Protocol != "http1" {
						t.Fatalf("sniffed content leaked or lost: %+v", got)
					}
					if target := session.OutboundsFromContext(outCtx)[0].Target.Address.String(); target != host {
						t.Fatalf("sniffed destination = %q, want %q", target, host)
					}
					if !got.SkipDNSResolve || !got.SniffingRequest.Enabled {
						t.Fatal("lost inherited sniff/DNS settings")
					}
					previous = got
				}
				if parent.Protocol != "" || len(parent.Attributes) != 0 {
					t.Fatal("dispatch mutated parent Content")
				}
			})
		}
	}
}

func TestDispatchContentPreservesAttributes(t *testing.T) {
	for _, method := range []string{"Dispatch", "DispatchLink"} {
		for _, sniff := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/sniff=%v", method, sniff), func(t *testing.T) {
				attrs := map[string]string{"forcedOutboundTag": "forced", ":method": "POST", ":path": "/original"}
				parent := &session.Content{Attributes: maps.Clone(attrs), Protocol: "http/1.1", SkipDNSResolve: true,
					SniffingRequest: session.SniffingRequest{Enabled: sniff}}
				ctx := contentTestContext(parent)
				results := make(chan context.Context, 2)
				h := &contentTestHandler{tag: "forced", inspect: func(ctx context.Context, link *transport.Link) { results <- ctx }}
				d := &DefaultDispatcher{ohm: &contentTestManager{handler: h}}
				for i := 0; i < 2; i++ {
					if err := runContentTestRequest(ctx, d, method, "GET /new HTTP/1.1\r\nHost: example.com\r\n\r\n"); err != nil {
						t.Fatal(err)
					}
					got := session.ContentFromContext(receiveContentTestResult(t, results))
					if got.Attribute("forcedOutboundTag") != "" {
						t.Fatal("forced outbound tag was not consumed")
					}
					if got.Attribute(":method") != "POST" || got.Attribute(":path") != "/original" {
						t.Fatal("lost HTTP inbound attributes")
					}
					got.SetAttribute("stream-only", "private")
					if !maps.Equal(parent.Attributes, attrs) {
						t.Fatalf("child mutated inherited attributes: %v", parent.Attributes)
					}
					if parent.Protocol != "http/1.1" {
						t.Fatal("child mutated inherited protocol")
					}
				}
			})
		}
	}
}

func TestDispatchContentConcurrentSniffAndRoute(t *testing.T) {
	for _, method := range []string{"Dispatch", "DispatchLink"} {
		t.Run(method, func(t *testing.T) {
			const streams = 64
			parent := &session.Content{SniffingRequest: session.SniffingRequest{Enabled: true}}
			ctx := contentTestContext(parent)
			results := make(chan context.Context, streams)
			h := &contentTestHandler{tag: "direct", inspect: func(ctx context.Context, link *transport.Link) {
				// Exercise the same read/write helpers used for forced routing.
				session.SetForcedOutboundTagToContext(ctx, "stream-only")
				if session.GetForcedOutboundTagFromContext(ctx) != "stream-only" {
					t.Error("lost route tag")
				}
				results <- ctx
			}}
			d := &DefaultDispatcher{ohm: &contentTestManager{handler: h}}
			start := make(chan struct{})
			var wg sync.WaitGroup
			for i := 0; i < streams; i++ {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()
					<-start
					payload := fmt.Sprintf("GET /%d HTTP/1.1\r\nHost: stream%d.example\r\nX-Stream: %d\r\n\r\n", i, i, i)
					if err := runContentTestRequest(ctx, d, method, payload); err != nil {
						t.Error(err)
					}
				}(i)
			}
			close(start)
			seen := make(map[string]bool)
			for i := 0; i < streams; i++ {
				content := session.ContentFromContext(receiveContentTestResult(t, results))
				host := content.Attribute("host")
				if host == "" || seen[host] {
					t.Errorf("missing or cross-stream HTTP host: %q", host)
				}
				seen[host] = true
			}
			wg.Wait()
			if parent.Protocol != "" || parent.Attributes != nil {
				t.Fatal("shared parent was mutated")
			}
		})
	}
}

func TestDispatchContentWithoutParent(t *testing.T) {
	for _, method := range []string{"Dispatch", "DispatchLink"} {
		t.Run(method, func(t *testing.T) {
			ctx := context.Background()
			results := make(chan context.Context, 1)
			h := &contentTestHandler{tag: "direct", inspect: func(ctx context.Context, link *transport.Link) { results <- ctx }}
			d := &DefaultDispatcher{ohm: &contentTestManager{handler: h}}
			if err := runContentTestRequest(ctx, d, method, "payload"); err != nil {
				t.Fatal(err)
			}
			if session.ContentFromContext(receiveContentTestResult(t, results)) == nil {
				t.Fatal("dispatch did not create Content")
			}
			if session.ContentFromContext(ctx) != nil {
				t.Fatal("dispatch modified caller context")
			}
		})
	}
}
