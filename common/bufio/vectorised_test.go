package bufio

import (
	"crypto/rand"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWriteVectorised(t *testing.T) {
	t.Parallel()
	inputConn, outputConn := TCPPipe(t)
	defer inputConn.Close()
	defer outputConn.Close()
	vectorisedWriter, created := CreateVectorisedWriter(inputConn)
	require.True(t, created)
	require.NotNil(t, vectorisedWriter)
	var bufA [1024]byte
	var bufB [1024]byte
	var bufC [2048]byte
	_, err := io.ReadFull(rand.Reader, bufA[:])
	require.NoError(t, err)
	_, err = io.ReadFull(rand.Reader, bufB[:])
	require.NoError(t, err)
	copy(bufC[:], bufA[:])
	copy(bufC[1024:], bufB[:])
	finish := Timeout(t)
	_, err = WriteVectorised(vectorisedWriter, [][]byte{bufA[:], bufB[:]})
	require.NoError(t, err)
	output := make([]byte, 2048)
	_, err = io.ReadFull(outputConn, output)
	finish()
	require.NoError(t, err)
	require.Equal(t, bufC[:], output)
}

func TestWriteVectorisedPacket(t *testing.T) {
	inputConn, outputConn, outputAddr := UDPPipe(t)
	defer inputConn.Close()
	defer outputConn.Close()
	vectorisedWriter, created := CreateVectorisedPacketWriter(inputConn)
	require.True(t, created)
	require.NotNil(t, vectorisedWriter)
	var bufA [1024]byte
	var bufB [1024]byte
	var bufC [2048]byte
	_, err := io.ReadFull(rand.Reader, bufA[:])
	require.NoError(t, err)
	_, err = io.ReadFull(rand.Reader, bufB[:])
	require.NoError(t, err)
	copy(bufC[:], bufA[:])
	copy(bufC[1024:], bufB[:])
	finish := Timeout(t)
	_, err = WriteVectorisedPacket(vectorisedWriter, [][]byte{bufA[:], bufB[:]}, outputAddr)
	require.NoError(t, err)
	output := make([]byte, 2048)
	n, _, err := outputConn.ReadFrom(output)
	finish()
	require.NoError(t, err)
	require.Equal(t, 2048, n)
	require.Equal(t, bufC[:], output)
}
