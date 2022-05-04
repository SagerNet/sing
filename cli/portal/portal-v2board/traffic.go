package main

import (
	"io"
	"net"
	"sync"
	"sync/atomic"

	"github.com/sagernet/sing/common/buf"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
)

type UserManager struct {
	access sync.Mutex
	users  map[int]*User
}

type User struct {
	Upload   uint64
	Download uint64
}

func NewUserManager() UserManager {
	return UserManager{
		users: make(map[int]*User),
	}
}

func (m *UserManager) TrackConnection(userId int, conn net.Conn) net.Conn {
	m.access.Lock()
	defer m.access.Unlock()
	var user *User
	if u, loaded := m.users[userId]; loaded {
		user = u
	} else {
		user = new(User)
		m.users[userId] = user
	}
	return &TrackConn{conn, user}
}

func (m *UserManager) TrackPacketConnection(userId int, conn N.PacketConn) N.PacketConn {
	m.access.Lock()
	defer m.access.Unlock()
	var user *User
	if u, loaded := m.users[userId]; loaded {
		user = u
	} else {
		user = new(User)
		m.users[userId] = user
	}
	return &TrackPacketConn{conn, user}
}

func (m *UserManager) ReadTraffics() []UserTraffic {
	m.access.Lock()
	defer m.access.Unlock()

	traffic := make([]UserTraffic, 0, len(m.users))
	for userId, user := range m.users {
		upload := atomic.SwapUint64(&user.Upload, 0)
		download := atomic.SwapUint64(&user.Download, 0)
		if upload == 0 && download == 0 {
			continue
		}
		traffic = append(traffic, UserTraffic{
			UID:      userId,
			Upload:   int64(upload),
			Download: int64(download),
		})
	}

	return traffic
}

type TrackConn struct {
	net.Conn
	*User
}

func (c *TrackConn) Read(p []byte) (n int, err error) {
	n, err = c.Conn.Read(p)
	if n > 0 {
		atomic.AddUint64(&c.Upload, uint64(n))
	}
	return
}

func (c *TrackConn) Write(p []byte) (n int, err error) {
	n, err = c.Conn.Write(p)
	if n > 0 {
		atomic.AddUint64(&c.Download, uint64(n))
	}
	return
}

func (c *TrackConn) WriteTo(w io.Writer) (n int64, err error) {
	n, err = io.Copy(w, c.Conn)
	if n > 0 {
		atomic.AddUint64(&c.Upload, uint64(n))
	}
	return
}

func (c *TrackConn) ReadFrom(r io.Reader) (n int64, err error) {
	n, err = io.Copy(c.Conn, r)
	if n > 0 {
		atomic.AddUint64(&c.Download, uint64(n))
	}
	return
}

type TrackPacketConn struct {
	N.PacketConn
	*User
}

func (c *TrackPacketConn) ReadPacket(buffer *buf.Buffer) (M.Socksaddr, error) {
	destination, err := c.PacketConn.ReadPacket(buffer)
	if err == nil {
		atomic.AddUint64(&c.Upload, uint64(buffer.Len()))
	}
	return destination, err
}

func (c *TrackPacketConn) WritePacket(buffer *buf.Buffer, destination M.Socksaddr) error {
	n := buffer.Len()
	err := c.PacketConn.WritePacket(buffer, destination)
	if err == nil {
		atomic.AddUint64(&c.Download, uint64(n))
	}
	return err
}
