package shadowsocks

import (
	"io"

	"github.com/sagernet/sing/common/buf"
)

func init() {
	RegisterCipher("none", func() Cipher {
		return (*NoneCipher)(nil)
	})
}

type NoneCipher struct{}

func (c *NoneCipher) KeySize() int {
	return 16
}

func (c *NoneCipher) SaltSize() int {
	return 0
}

func (c *NoneCipher) CreateReader(_ []byte, _ []byte, reader io.Reader) io.Reader {
	return reader
}

func (c *NoneCipher) CreateWriter(_ []byte, _ []byte, writer io.Writer) (io.Writer, int) {
	return writer, 0
}

func (c *NoneCipher) EncodePacket([]byte, *buf.Buffer) error {
	return nil
}

func (c *NoneCipher) DecodePacket([]byte, *buf.Buffer) error {
	return nil
}
