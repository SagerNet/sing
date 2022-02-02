package shadowsocks

import (
	"io"

	"sing/common/buf"
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

func (c *NoneCipher) IVSize() int {
	return 0
}

func (c *NoneCipher) NewEncryptionWriter(_ []byte, _ []byte, writer io.Writer) (io.Writer, error) {
	return writer, nil
}

func (c *NoneCipher) NewDecryptionReader(_ []byte, _ []byte, reader io.Reader) (io.Reader, error) {
	return reader, nil
}

func (c *NoneCipher) EncodePacket([]byte, *buf.Buffer) error {
	return nil
}

func (c *NoneCipher) DecodePacket([]byte, *buf.Buffer) error {
	return nil
}
