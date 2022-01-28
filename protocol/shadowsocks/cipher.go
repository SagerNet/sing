package shadowsocks

import (
	"bytes"
	"io"

	"sing/common/exceptions"
)

type Cipher interface {
	KeySize() int
	IVSize() int
	NewEncryptionWriter(key []byte, iv []byte, writer io.Writer) (io.Writer, error)
	NewDecryptionReader(key []byte, iv []byte, reader io.Reader) (io.Reader, error)
	EncodePacket(key []byte, buffer *bytes.Buffer) error
	DecodePacket(key []byte, buffer *bytes.Buffer) error
}

type CipherCreator func() Cipher

var cipherList map[string]CipherCreator

func init() {
	cipherList = make(map[string]CipherCreator)
}

func RegisterCipher(method string, creator CipherCreator) {
	cipherList[method] = creator
}

func CreateCipher(method string) (Cipher, error) {
	creator := cipherList[method]
	if creator != nil {
		return creator(), nil
	}
	return nil, exceptions.New("unsupported method: ", method)
}
