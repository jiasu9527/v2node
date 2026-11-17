package dispatcher

import (
	"sync"
	"testing"
	"time"

	"github.com/xtls/xray-core/common"
	"github.com/xtls/xray-core/common/buf"
	"github.com/xtls/xray-core/transport/pipe"
)

func TestUoTLifecycle(t *testing.T) {
	for _, kind := range []string{"decode", "encode"} {
		for _, operation := range []string{"Close", "Interrupt"} {
			t.Run(kind+"/"+operation, func(t *testing.T) {
				r, w := pipe.New()
				defer r.Interrupt()
				defer w.Interrupt()
				var wrapped buf.Writer
				if kind == "decode" {
					wrapped = newAnyTLSUoTDecodeWriter(w, &anyTLSUoTState{})
				} else {
					wrapped = newAnyTLSUoTEncodeWriter(w, &anyTLSUoTState{})
				}
				done := make(chan error, 1)
				go func() { mb, err := r.ReadMultiBuffer(); buf.ReleaseMulti(mb); done <- err }()
				if operation == "Close" {
					_ = common.Close(wrapped)
				} else {
					_ = common.Interrupt(wrapped)
				}
				select {
				case err := <-done:
					if err == nil {
						t.Fatal("reader returned without closed error")
					}
				case <-time.After(time.Second):
					t.Error("wrapped writer did not propagate lifecycle; pipe reader remains blocked")
				}
			})
		}
	}
}

func TestLinkManagerCloseAll(t *testing.T) {
	r, w := pipe.New()
	defer r.Interrupt()
	defer w.Interrupt()
	m := &LinkManager{links: make(map[*ManagedWriter]buf.Reader)}
	mw := &ManagedWriter{writer: w, manager: m}
	m.AddLink(mw, r)
	done := make(chan struct{})
	go func() { m.CloseAll(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Error("CloseAll deadlocks: holds manager mutex while ManagedWriter.Close reacquires it")
	}
}

func TestLinkManagerConcurrentCloseAndAdd(t *testing.T) {
	m := &LinkManager{}
	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, w := pipe.New()
			defer r.Interrupt()
			defer w.Interrupt()
			mw := &ManagedWriter{writer: w, manager: m}
			m.AddLink(mw, r)
			m.CloseAll()
			mw.Close()
			mw.Interrupt()
		}()
	}
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("concurrent close/add deadlocked")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.links) != 0 {
		t.Fatal("links survived CloseAll")
	}
}

func TestLinkManagerRejectsLateLink(t *testing.T) {
	m := &LinkManager{}
	m.CloseAll()
	r, w := pipe.New()
	defer r.Interrupt()
	defer w.Interrupt()
	m.AddLink(&ManagedWriter{writer: w, manager: m}, r)
	mb, err := r.ReadMultiBufferTimeout(time.Second)
	buf.ReleaseMulti(mb)
	if err == nil || err == buf.ErrReadTimeout {
		t.Fatalf("late pipe not interrupted: %v", err)
	}
}

func TestUoTCloseDrainsBufferedPacket(t *testing.T) {
	for _, encode := range []bool{false, true} {
		r, w := pipe.New()
		defer r.Interrupt()
		defer w.Interrupt()
		state := &anyTLSUoTState{}
		var wrapped buf.Writer
		input := []byte{0, 3, 'd', 'n', 's'}
		want := []byte("dns")
		if encode {
			wrapped = newAnyTLSUoTEncodeWriter(w, state)
			input, want = want, input
		} else {
			wrapped = newAnyTLSUoTDecodeWriter(w, state)
		}
		if err := wrapped.WriteMultiBuffer(buf.MergeBytes(nil, input)); err != nil {
			t.Fatal(err)
		}
		if err := common.Close(wrapped); err != nil {
			t.Fatal(err)
		}
		mb, err := r.ReadMultiBufferTimeout(time.Second)
		if err != nil {
			t.Fatal(err)
		}
		got := make([]byte, mb.Len())
		mb.Copy(got)
		buf.ReleaseMulti(mb)
		if string(got) != string(want) {
			t.Fatalf("data lost: %v != %v", got, want)
		}
		mb, err = r.ReadMultiBufferTimeout(time.Second)
		buf.ReleaseMulti(mb)
		if err == nil || err == buf.ErrReadTimeout {
			t.Fatalf("missing EOF: %v", err)
		}
	}
}

func TestUoTInterruptUnblocksBackpressuredWriter(t *testing.T) {
	for _, encode := range []bool{false, true} {
		r, w := pipe.New(pipe.WithSizeLimit(0))
		defer r.Interrupt()
		defer w.Interrupt()
		state := &anyTLSUoTState{}
		var wrapped buf.Writer
		input := []byte{0, 3, 'd', 'n', 's'}
		if encode {
			wrapped = newAnyTLSUoTEncodeWriter(w, state)
			input = []byte("dns")
		} else {
			wrapped = newAnyTLSUoTDecodeWriter(w, state)
		}
		if err := wrapped.WriteMultiBuffer(buf.MergeBytes(nil, input)); err != nil {
			t.Fatal(err)
		}
		done := make(chan error, 1)
		go func() { done <- wrapped.WriteMultiBuffer(buf.MergeBytes(nil, input)) }()
		common.Interrupt(wrapped)
		select {
		case err := <-done:
			if err == nil {
				t.Fatal("write succeeded after interrupt")
			}
		case <-time.After(time.Second):
			t.Fatal("writer remains blocked")
		}
	}
}
