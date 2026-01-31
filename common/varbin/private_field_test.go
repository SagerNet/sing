package varbin

import (
	"bytes"
	"testing"

	"github.com/sagernet/sing/common/binary"
	"github.com/stretchr/testify/require"
)

func TestReadPrivateFieldReturnsError(t *testing.T) {
	t.Parallel()

	type sample struct {
		Exported string
		private  string
	}

	buildBytes := func() []byte {
		var buffer bytes.Buffer
		_, err := WriteUvarint(&buffer, 1)
		require.NoError(t, err)
		_, err = buffer.WriteString("a")
		require.NoError(t, err)
		_, err = WriteUvarint(&buffer, 0)
		require.NoError(t, err)
		return buffer.Bytes()
	}

	require.NotPanics(t, func() {
		var value sample
		reader := bytes.NewBuffer(buildBytes())
		err := Read(reader, binary.BigEndian, &value)
		require.Error(t, err)
	})
}

func TestWriteStructValueMatchesPointer(t *testing.T) {
	t.Parallel()

	type sample struct {
		Name  string
		Ports []uint16
	}

	value := sample{
		Name:  "alpha",
		Ports: []uint16{80, 443},
	}

	var valueBuffer bytes.Buffer
	err := Write(&valueBuffer, binary.BigEndian, value)
	require.NoError(t, err)

	var pointerBuffer bytes.Buffer
	err = Write(&pointerBuffer, binary.BigEndian, &value)
	require.NoError(t, err)

	require.Equal(t, pointerBuffer.Bytes(), valueBuffer.Bytes())
}
