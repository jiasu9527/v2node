package anytls

import (
	"sync"

	"github.com/xtls/xray-core/common"
	xnet "github.com/xtls/xray-core/common/net"
	"github.com/xtls/xray-core/transport"
)

type stream struct {
	sid  uint32
	link *transport.Link

	done     chan struct{}
	doneOnce sync.Once
	errMu    sync.Mutex
	err      error
	closed   bool
	dieHook  func()

	isUDP     bool
	udpTarget *xnet.Destination
}

func newStream(sid uint32, link *transport.Link) *stream {
	return &stream{
		sid:  sid,
		link: link,
		done: make(chan struct{}),
	}
}

// getLink/setLink serialize link publication with session shutdown. A dispatch
// may finish after Close has already detached the stream from the session.
func (st *stream) getLink() *transport.Link {
	st.errMu.Lock()
	defer st.errMu.Unlock()
	return st.link
}

func (st *stream) setLink(link *transport.Link) bool {
	st.errMu.Lock()
	if st.closed {
		st.errMu.Unlock()
		common.Interrupt(link.Reader)
		common.Interrupt(link.Writer)
		return false
	}
	st.link = link
	st.errMu.Unlock()
	return true
}

func (st *stream) close(err error) { st.finish(err, false) }
func (st *stream) abort(err error) { st.finish(err, true) }

func (st *stream) finish(err error, abort bool) {
	st.doneOnce.Do(func() {
		st.errMu.Lock()
		st.err = err
		st.closed = true
		link := st.link
		st.errMu.Unlock()
		if link != nil {
			// pipe.Reader implements Interrupt, not Close. Always wake its pump.
			common.Interrupt(link.Reader)
			if abort {
				common.Interrupt(link.Writer)
			} else {
				common.Close(link.Writer)
			}
		}
		if st.done != nil {
			close(st.done)
		}
		if st.dieHook != nil {
			st.dieHook()
		}
	})
}

func (st *stream) result() error {
	st.errMu.Lock()
	defer st.errMu.Unlock()
	return st.err
}
