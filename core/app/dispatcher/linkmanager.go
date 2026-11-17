package dispatcher

import (
	"sync"

	"github.com/xtls/xray-core/common"
	"github.com/xtls/xray-core/common/buf"
)

type ManagedWriter struct {
	writer  buf.Writer
	manager *LinkManager
}

func (w *ManagedWriter) WriteMultiBuffer(mb buf.MultiBuffer) error {
	return w.writer.WriteMultiBuffer(mb)
}

func (w *ManagedWriter) Close() error {
	w.manager.RemoveWriter(w)
	return common.Close(w.writer)
}

func (w *ManagedWriter) Interrupt() {
	w.manager.RemoveWriter(w)
	common.Interrupt(w.writer)
}

type LinkManager struct {
	links  map[*ManagedWriter]buf.Reader
	mu     sync.RWMutex
	closed bool
}

func (m *LinkManager) AddLink(writer *ManagedWriter, reader buf.Reader) {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		common.Interrupt(writer)
		common.Interrupt(reader)
		return
	}
	if m.links == nil {
		m.links = make(map[*ManagedWriter]buf.Reader)
	}
	m.links[writer] = reader
	m.mu.Unlock()
}

func (m *LinkManager) RemoveWriter(writer *ManagedWriter) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.links, writer)
}

func (m *LinkManager) CloseAll() {
	m.mu.Lock()
	// Detach under the lock, close outside it: ManagedWriter calls RemoveWriter.
	// A stale manager must also reject links racing with user deletion.
	m.closed = true
	links := m.links
	m.links = nil
	m.mu.Unlock()
	for w, r := range links {
		common.Interrupt(w)
		common.Interrupt(r)
	}
}
