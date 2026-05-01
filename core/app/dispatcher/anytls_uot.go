package dispatcher

import (
	"context"
	"encoding/binary"
	"fmt"
	"strings"

	"github.com/xtls/xray-core/common/buf"
	"github.com/xtls/xray-core/common/net"
	"github.com/xtls/xray-core/common/session"
	"github.com/xtls/xray-core/transport"
)

const maxAnyTLSUoTPendingBytes = 1 << 20

type anyTLSUoTDecodeWriter struct {
	writer   buf.Writer
	pending  []byte
	expected int
}

func newAnyTLSUoTDecodeWriter(writer buf.Writer) buf.Writer {
	return &anyTLSUoTDecodeWriter{
		writer:   writer,
		expected: -1,
	}
}

func (w *anyTLSUoTDecodeWriter) WriteMultiBuffer(mb buf.MultiBuffer) error {
	if mb.IsEmpty() {
		buf.ReleaseMulti(mb)
		return nil
	}
	payload := make([]byte, mb.Len())
	mb.Copy(payload)
	buf.ReleaseMulti(mb)

	w.pending = append(w.pending, payload...)
	if len(w.pending) > maxAnyTLSUoTPendingBytes {
		return fmt.Errorf("anytls uot pending buffer too large: %d", len(w.pending))
	}

	for {
		if w.expected < 0 {
			if len(w.pending) < 2 {
				return nil
			}
			w.expected = int(binary.BigEndian.Uint16(w.pending[:2]))
			w.pending = w.pending[2:]
			if w.expected == 0 {
				w.expected = -1
				continue
			}
		}
		if len(w.pending) < w.expected {
			return nil
		}
		packet := w.pending[:w.expected]
		w.pending = w.pending[w.expected:]
		w.expected = -1
		if err := w.writer.WriteMultiBuffer(buf.MergeBytes(nil, packet)); err != nil {
			return err
		}
	}
}

type anyTLSUoTEncodeWriter struct {
	writer buf.Writer
}

func newAnyTLSUoTEncodeWriter(writer buf.Writer) buf.Writer {
	return &anyTLSUoTEncodeWriter{writer: writer}
}

func (w *anyTLSUoTEncodeWriter) WriteMultiBuffer(mb buf.MultiBuffer) error {
	if mb.IsEmpty() {
		buf.ReleaseMulti(mb)
		return nil
	}
	for _, b := range mb {
		if b != nil && b.Len() > 0xffff {
			buf.ReleaseMulti(mb)
			return fmt.Errorf("anytls uot packet too large: %d", b.Len())
		}
	}
	var framed buf.MultiBuffer
	for _, b := range mb {
		if b == nil {
			continue
		}
		if b.IsEmpty() {
			b.Release()
			continue
		}
		header := buf.NewWithSize(2)
		binary.BigEndian.PutUint16(header.Extend(2), uint16(b.Len()))
		framed = append(framed, header, b)
	}
	if framed.IsEmpty() {
		return nil
	}
	return w.writer.WriteMultiBuffer(framed)
}

func wrapAnyTLSUoTLinks(ctx context.Context, network net.Network, inbound, outbound *transport.Link) {
	if network != net.Network_UDP || inbound == nil || outbound == nil {
		return
	}
	inboundSession := session.InboundFromContext(ctx)
	if inboundSession == nil || !strings.EqualFold(inboundSession.Name, "anytls") {
		return
	}

	// Xray's AnyTLS inbound parses the initial UoT request but passes later
	// datagram frames through as raw bytes. Normalize them here so standard
	// UoT clients such as sing-box/V2bX-compatible clients keep UDP semantics.
	inbound.Writer = newAnyTLSUoTDecodeWriter(inbound.Writer)
	outbound.Writer = newAnyTLSUoTEncodeWriter(outbound.Writer)
}
