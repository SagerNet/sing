package conntrack

import (
	"io"
	"sync"

	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/x/list"
)

type Tracker struct {
	access      sync.Mutex
	connections list.List[io.Closer]
}

func (m *Tracker) Track(conn io.Closer) *Registration {
	m.access.Lock()
	element := m.connections.PushBack(conn)
	m.access.Unlock()
	return &Registration{m, element}
}

func (m *Tracker) Reset() {
	m.access.Lock()
	defer m.access.Unlock()
	for element := m.connections.Front(); element != nil; element = element.Next() {
		common.Close(element.Value)
	}
	m.connections = list.List[io.Closer]{}
}

type Registration struct {
	manager *Tracker
	element *list.Element[io.Closer]
}

func (t *Registration) Leave() {
	t.manager.access.Lock()
	defer t.manager.access.Unlock()
	t.manager.connections.Remove(t.element)
}
