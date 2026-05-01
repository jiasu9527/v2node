package dispatcher

import (
	"bytes"
	"encoding/binary"
	"testing"

	M "github.com/sagernet/sing/common/metadata"
	"github.com/sagernet/sing/common/uot"
	"github.com/xtls/xray-core/common/buf"
	xnet "github.com/xtls/xray-core/common/net"
)

type recordingMultiBufferWriter struct {
	payloads [][]byte
}

func (w *recordingMultiBufferWriter) WriteMultiBuffer(mb buf.MultiBuffer) error {
	defer buf.ReleaseMulti(mb)
	for _, b := range mb {
		if b == nil || b.IsEmpty() {
			continue
		}
		w.payloads = append(w.payloads, append([]byte(nil), b.Bytes()...))
	}
	return nil
}

func multiBufferFromBytes(payloads ...[]byte) buf.MultiBuffer {
	var mb buf.MultiBuffer
	for _, payload := range payloads {
		b := buf.NewWithSize(int32(len(payload)))
		_, _ = b.Write(payload)
		mb = append(mb, b)
	}
	return mb
}

func uotPacket(payload []byte) []byte {
	packet := make([]byte, 2+len(payload))
	binary.BigEndian.PutUint16(packet[:2], uint16(len(payload)))
	copy(packet[2:], payload)
	return packet
}

func TestAnyTLSUoTDecodeWriterDeframesPackets(t *testing.T) {
	underlying := &recordingMultiBufferWriter{}
	writer := newAnyTLSUoTDecodeWriter(underlying, &anyTLSUoTState{})

	first := append(uotPacket([]byte("hello")), uotPacket([]byte("world"))[:3]...)
	second := uotPacket([]byte("world"))[3:]
	if err := writer.WriteMultiBuffer(multiBufferFromBytes(first[:4], first[4:], second)); err != nil {
		t.Fatal(err)
	}

	want := [][]byte{[]byte("hello"), []byte("world")}
	if len(underlying.payloads) != len(want) {
		t.Fatalf("payload count = %d, want %d", len(underlying.payloads), len(want))
	}
	for i := range want {
		if !bytes.Equal(underlying.payloads[i], want[i]) {
			t.Fatalf("payload[%d] = %q, want %q", i, underlying.payloads[i], want[i])
		}
	}
}

func TestAnyTLSUoTEncodeWriterFramesPackets(t *testing.T) {
	underlying := &recordingMultiBufferWriter{}
	writer := newAnyTLSUoTEncodeWriter(underlying, &anyTLSUoTState{})

	if err := writer.WriteMultiBuffer(multiBufferFromBytes([]byte("hello"), []byte("world"))); err != nil {
		t.Fatal(err)
	}

	got := bytes.Join(underlying.payloads, nil)
	want := append(uotPacket([]byte("hello")), uotPacket([]byte("world"))...)
	if !bytes.Equal(got, want) {
		t.Fatalf("encoded payload = %v, want %v", got, want)
	}
}

func TestAnyTLSUoTDecodeWriterDeframesPacketMode(t *testing.T) {
	underlying := &recordingMultiBufferWriter{}
	writer := newAnyTLSUoTDecodeWriter(underlying, &anyTLSUoTState{})

	frame := uotPacketWithDestination("1.1.1.1:53", []byte("dns-query"))
	if err := writer.WriteMultiBuffer(multiBufferFromBytes(frame)); err != nil {
		t.Fatal(err)
	}

	if len(underlying.payloads) != 1 {
		t.Fatalf("payload count = %d, want 1", len(underlying.payloads))
	}
	if !bytes.Equal(underlying.payloads[0], []byte("dns-query")) {
		t.Fatalf("payload = %q, want dns-query", underlying.payloads[0])
	}
}

func TestAnyTLSUoTEncodeWriterFramesPacketMode(t *testing.T) {
	underlying := &recordingMultiBufferWriter{}
	state := &anyTLSUoTState{}
	state.setMode(anyTLSUoTModePacket)
	writer := newAnyTLSUoTEncodeWriter(underlying, state)

	mb := multiBufferFromBytes([]byte("dns-response"))
	mb[0].UDP = &xnet.Destination{
		Network: xnet.Network_UDP,
		Address: xnet.ParseAddress("1.1.1.1"),
		Port:    xnet.Port(53),
	}
	if err := writer.WriteMultiBuffer(mb); err != nil {
		t.Fatal(err)
	}

	got := bytes.Join(underlying.payloads, nil)
	want := uotPacketWithDestination("1.1.1.1:53", []byte("dns-response"))
	if !bytes.Equal(got, want) {
		t.Fatalf("encoded packet frame = %v, want %v", got, want)
	}
}

func uotPacketWithDestination(destination string, payload []byte) []byte {
	addr := buf.New()
	if err := uot.AddrParser.WriteAddrPort(addr, M.ParseSocksaddr(destination)); err != nil {
		panic(err)
	}
	defer addr.Release()
	packet := append([]byte(nil), addr.Bytes()...)
	return append(packet, uotPacket(payload)...)
}
