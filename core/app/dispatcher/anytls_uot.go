package dispatcher

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"strings"
	"sync"

	"github.com/sagernet/sing/common/uot"
	"github.com/xtls/xray-core/common/buf"
	"github.com/xtls/xray-core/common/net"
	"github.com/xtls/xray-core/common/session"
	"github.com/xtls/xray-core/common/singbridge"
	"github.com/xtls/xray-core/transport"
)

const maxAnyTLSUoTPendingBytes = 1 << 20

type anyTLSUoTMode int

const (
	anyTLSUoTModeUnknown anyTLSUoTMode = iota
	anyTLSUoTModeConnect
	anyTLSUoTModePacket
)

type anyTLSUoTState struct {
	sync.RWMutex
	mode anyTLSUoTMode
}

func (s *anyTLSUoTState) getMode() anyTLSUoTMode {
	s.RLock()
	defer s.RUnlock()
	return s.mode
}

func (s *anyTLSUoTState) setMode(mode anyTLSUoTMode) {
	s.Lock()
	if s.mode == anyTLSUoTModeUnknown {
		s.mode = mode
	}
	s.Unlock()
}

type anyTLSUoTDecodeWriter struct {
	writer  buf.Writer
	state   *anyTLSUoTState
	pending []byte
}

func newAnyTLSUoTDecodeWriter(writer buf.Writer, state *anyTLSUoTState) buf.Writer {
	return &anyTLSUoTDecodeWriter{
		writer: writer,
		state:  state,
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

	for len(w.pending) > 0 {
		mode := w.state.getMode()
		if mode == anyTLSUoTModeUnknown {
			connectComplete := anyTLSUoTConnectFramesComplete(w.pending)
			packetComplete := anyTLSUoTPacketFramesComplete(w.pending)
			switch {
			case packetComplete && !connectComplete:
				w.state.setMode(anyTLSUoTModePacket)
				mode = anyTLSUoTModePacket
			case connectComplete:
				w.state.setMode(anyTLSUoTModeConnect)
				mode = anyTLSUoTModeConnect
			default:
				return nil
			}
		}

		var (
			mb      buf.MultiBuffer
			remain  []byte
			decoded bool
		)
		if mode == anyTLSUoTModePacket {
			var err error
			mb, remain, decoded, err = decodeAnyTLSUoTPacketFrame(w.pending)
			if err != nil {
				return err
			}
		} else {
			mb, remain, decoded = decodeAnyTLSUoTConnectFrame(w.pending)
		}
		if !decoded {
			return nil
		}
		w.pending = remain
		if mb.IsEmpty() {
			continue
		}
		if err := w.writer.WriteMultiBuffer(mb); err != nil {
			return err
		}
	}
	return nil
}

type anyTLSUoTEncodeWriter struct {
	writer buf.Writer
	state  *anyTLSUoTState
}

func newAnyTLSUoTEncodeWriter(writer buf.Writer, state *anyTLSUoTState) buf.Writer {
	return &anyTLSUoTEncodeWriter{
		writer: writer,
		state:  state,
	}
}

func (w *anyTLSUoTEncodeWriter) WriteMultiBuffer(mb buf.MultiBuffer) error {
	if mb.IsEmpty() {
		buf.ReleaseMulti(mb)
		return nil
	}
	mode := w.state.getMode()
	for _, b := range mb {
		if b != nil && b.Len() > 0xffff {
			buf.ReleaseMulti(mb)
			return fmt.Errorf("anytls uot packet too large: %d", b.Len())
		}
	}
	var framed buf.MultiBuffer
	for {
		remaining, b := buf.SplitFirst(mb)
		mb = remaining
		if b == nil {
			break
		}
		if b.IsEmpty() {
			b.Release()
			continue
		}
		if mode == anyTLSUoTModePacket && b.UDP != nil {
			addr := buf.New()
			if err := uot.AddrParser.WriteAddrPort(addr, singbridge.ToSocksaddr(*b.UDP)); err != nil {
				addr.Release()
				b.Release()
				buf.ReleaseMulti(mb)
				buf.ReleaseMulti(framed)
				return err
			}
			framed = append(framed, addr)
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
	state := &anyTLSUoTState{}
	inbound.Writer = newAnyTLSUoTDecodeWriter(inbound.Writer, state)
	outbound.Writer = newAnyTLSUoTEncodeWriter(outbound.Writer, state)
}

func decodeAnyTLSUoTConnectFrame(raw []byte) (buf.MultiBuffer, []byte, bool) {
	if len(raw) < 2 {
		return nil, raw, false
	}
	length := int(binary.BigEndian.Uint16(raw[:2]))
	if len(raw) < 2+length {
		return nil, raw, false
	}
	if length == 0 {
		return nil, raw[2:], true
	}
	return buf.MergeBytes(nil, raw[2:2+length]), raw[2+length:], true
}

func decodeAnyTLSUoTPacketFrame(raw []byte) (buf.MultiBuffer, []byte, bool, error) {
	destination, payloadStart, ok := parseAnyTLSUoTPacketDestination(raw)
	if !ok {
		return nil, raw, false, nil
	}
	if len(raw[payloadStart:]) < 2 {
		return nil, raw, false, nil
	}
	length := int(binary.BigEndian.Uint16(raw[payloadStart : payloadStart+2]))
	frameEnd := payloadStart + 2 + length
	if len(raw) < frameEnd {
		return nil, raw, false, nil
	}
	if length == 0 {
		return nil, raw[frameEnd:], true, nil
	}
	mb := buf.MergeBytes(nil, raw[payloadStart+2:frameEnd])
	if len(mb) > 0 {
		mb[0].UDP = &destination
	}
	return mb, raw[frameEnd:], true, nil
}

func anyTLSUoTConnectFramesComplete(raw []byte) bool {
	if len(raw) == 0 {
		return false
	}
	for len(raw) > 0 {
		var decoded bool
		var mb buf.MultiBuffer
		mb, raw, decoded = decodeAnyTLSUoTConnectFrame(raw)
		buf.ReleaseMulti(mb)
		if !decoded {
			return false
		}
	}
	return true
}

func anyTLSUoTPacketFramesComplete(raw []byte) bool {
	if len(raw) == 0 {
		return false
	}
	for len(raw) > 0 {
		var decoded bool
		var err error
		var mb buf.MultiBuffer
		mb, raw, decoded, err = decodeAnyTLSUoTPacketFrame(raw)
		buf.ReleaseMulti(mb)
		if err != nil || !decoded {
			return false
		}
	}
	return true
}

func parseAnyTLSUoTPacketDestination(raw []byte) (net.Destination, int, bool) {
	destination, err := uot.AddrParser.ReadAddrPort(bytes.NewReader(raw))
	if err != nil {
		return net.Destination{}, 0, false
	}
	headerLen := uot.AddrParser.AddrPortLen(destination)
	if headerLen <= 0 || len(raw) < headerLen {
		return net.Destination{}, 0, false
	}
	target := singbridge.ToDestination(destination, net.Network_UDP)
	if !target.IsValid() {
		return net.Destination{}, 0, false
	}
	return target, headerLen, true
}
