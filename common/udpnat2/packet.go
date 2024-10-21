package udpnat

import (
	M "github.com/sagernet/sing/common/metadata"
	"sync"

	"github.com/sagernet/sing/common/buf"
)

var packetPool = sync.Pool{
	New: func() any {
		return new(Packet)
	},
}

type Packet struct {
	Buffer      *buf.Buffer
	Destination M.Socksaddr
}

func NewPacket() *Packet {
	return packetPool.Get().(*Packet)
}

func PutPacket(packet *Packet) {
	*packet = Packet{}
	packetPool.Put(packet)
}
