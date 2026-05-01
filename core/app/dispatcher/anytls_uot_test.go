package dispatcher

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/xtls/xray-core/common/buf"
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
	writer := newAnyTLSUoTDecodeWriter(underlying)

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
	writer := newAnyTLSUoTEncodeWriter(underlying)

	if err := writer.WriteMultiBuffer(multiBufferFromBytes([]byte("hello"), []byte("world"))); err != nil {
		t.Fatal(err)
	}

	got := bytes.Join(underlying.payloads, nil)
	want := append(uotPacket([]byte("hello")), uotPacket([]byte("world"))...)
	if !bytes.Equal(got, want) {
		t.Fatalf("encoded payload = %v, want %v", got, want)
	}
}
