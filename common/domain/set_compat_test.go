package domain_test

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/sagernet/sing/common/domain"
	"github.com/sagernet/sing/common/varbin"

	"github.com/stretchr/testify/require"
)

type succinctSetData struct {
	Reserved    uint8
	Leaves      []uint64
	LabelBitmap []uint64
	Labels      []byte
}

func legacyReadSuccinctSetData(data []byte) (succinctSetData, error) {
	return varbin.ReadValue[succinctSetData](bytes.NewReader(data), binary.BigEndian) //nolint:staticcheck
}

func legacyWriteSuccinctSetData(data succinctSetData) ([]byte, error) {
	var buf bytes.Buffer
	err := varbin.Write(&buf, binary.BigEndian, data) //nolint:staticcheck
	return buf.Bytes(), err
}

func TestSuccinctSetCompat(t *testing.T) {
	t.Parallel()
	testDomain := []string{"example.com", "example.org"}
	testDomainSuffix := []string{".com.cn", ".org.cn", "sagernet.org"}
	matcher := domain.NewMatcher(testDomain, testDomainSuffix, false)

	var newBuf bytes.Buffer
	require.NoError(t, matcher.Write(&newBuf))
	newBytes := newBuf.Bytes()

	data, err := legacyReadSuccinctSetData(newBytes)
	require.NoError(t, err)

	legacyBytes, err := legacyWriteSuccinctSetData(data)
	require.NoError(t, err)

	require.Equal(t, newBytes, legacyBytes)

	restored, err := domain.ReadMatcher(bytes.NewReader(legacyBytes))
	require.NoError(t, err)

	require.True(t, restored.Match("example.com"))
	require.True(t, restored.Match("example.org"))
	require.False(t, restored.Match("example.cn"))
	require.True(t, restored.Match("example.com.cn"))
	require.True(t, restored.Match("example.org.cn"))
	require.True(t, restored.Match("sagernet.org"))
	require.True(t, restored.Match("sing-box.sagernet.org"))
	dDomain, dDomainSuffix := restored.Dump()
	require.Equal(t, testDomain, dDomain)
	require.Equal(t, testDomainSuffix, dDomainSuffix)
}

func TestSuccinctSetCompatLegacy(t *testing.T) {
	t.Parallel()
	testDomain := []string{"example.com", "example.org"}
	testDomainSuffix := []string{".com.cn", ".org.cn", "sagernet.org"}
	matcher := domain.NewMatcher(testDomain, testDomainSuffix, true)

	var newBuf bytes.Buffer
	require.NoError(t, matcher.Write(&newBuf))
	newBytes := newBuf.Bytes()

	data, err := legacyReadSuccinctSetData(newBytes)
	require.NoError(t, err)

	legacyBytes, err := legacyWriteSuccinctSetData(data)
	require.NoError(t, err)

	require.Equal(t, newBytes, legacyBytes)

	restored, err := domain.ReadMatcher(bytes.NewReader(legacyBytes))
	require.NoError(t, err)

	require.True(t, restored.Match("example.com"))
	require.True(t, restored.Match("example.org"))
	require.False(t, restored.Match("example.cn"))
	require.True(t, restored.Match("example.com.cn"))
	require.True(t, restored.Match("example.org.cn"))
	require.True(t, restored.Match("sagernet.org"))
	require.True(t, restored.Match("sing-box.sagernet.org"))
	dDomain, dDomainSuffix := restored.Dump()
	require.Equal(t, testDomain, dDomain)
	require.Equal(t, testDomainSuffix, dDomainSuffix)
}
