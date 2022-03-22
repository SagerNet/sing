package shadowsocks

import (
	"io"

	"sing/common/buf"
	"sing/common/exceptions"
	"sing/common/list"
)

type Cipher interface {
	KeySize() int
	IVSize() int
	NewEncryptionWriter(key []byte, iv []byte, writer io.Writer) (io.Writer, error)
	NewDecryptionReader(key []byte, iv []byte, reader io.Reader) (io.Reader, error)
	EncodePacket(key []byte, buffer *buf.Buffer) error
	DecodePacket(key []byte, buffer *buf.Buffer) error
}

type CipherCreator func() Cipher

var (
	cipherList *list.List[string]
	cipherMap  map[string]CipherCreator
)

func init() {
	cipherList = new(list.List[string])
	cipherMap = make(map[string]CipherCreator)
}

func RegisterCipher(method string, creator CipherCreator) {
	cipherList.PushBack(method)
	cipherMap[method] = creator
}

func CreateCipher(method string) (Cipher, error) {
	creator := cipherMap[method]
	if creator != nil {
		return creator(), nil
	}
	return nil, exceptions.New("unsupported method: ", method)
}

func ListCiphers() []string {
	return cipherList.Array()
}
