package shadowaead_2022_test

import (
	"context"
	"net"
	"sync"
	"testing"

	"github.com/sagernet/sing/common"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
	"github.com/sagernet/sing/common/random"
	"github.com/sagernet/sing/protocol/shadowsocks/shadowaead_2022"
)

func TestMultiService(t *testing.T) {
	method := "2022-blake3-aes-128-gcm"
	var iPSK [16]byte
	random.Default.Read(iPSK[:])

	var wg sync.WaitGroup

	multiService, err := shadowaead_2022.NewMultiService[string](method, iPSK[:], random.Default, 500, &multiHandler{t, &wg})
	if err != nil {
		t.Fatal(err)
	}

	var uPSK [16]byte
	random.Default.Read(uPSK[:])
	multiService.AddUser("my user", uPSK[:])

	client, err := shadowaead_2022.New(method, [][]byte{iPSK[:], uPSK[:]}, "", random.Default)
	if err != nil {
		t.Fatal(err)
	}
	wg.Add(1)

	serverConn, clientConn := net.Pipe()
	defer common.Close(serverConn, clientConn)
	go func() {
		err := multiService.NewConnection(context.Background(), serverConn, M.Metadata{})
		if err != nil {
			t.Error(err)
			return
		}
	}()
	_, err = client.DialConn(clientConn, M.ParseSocksaddr("test.com:443"))
	if err != nil {
		t.Fatal(err)
	}
	wg.Wait()
}

type multiHandler struct {
	t  *testing.T
	wg *sync.WaitGroup
}

func (h *multiHandler) NewConnection(ctx context.Context, conn net.Conn, metadata M.Metadata) error {
	if metadata.Destination.String() != "test.com:443" {
		h.t.Error("bad destination")
	}
	h.wg.Done()
	return nil
}

func (h *multiHandler) NewPacketConnection(ctx context.Context, conn N.PacketConn, metadata M.Metadata) error {
	return nil
}

func (h *multiHandler) HandleError(err error) {
	h.t.Error(err)
}
