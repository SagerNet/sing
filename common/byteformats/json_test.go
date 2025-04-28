package byteformats_test

import (
	"testing"

	"github.com/sagernet/sing/common/byteformats"
	"github.com/sagernet/sing/common/json"

	"github.com/stretchr/testify/require"
)

func TestNetworkBytes(t *testing.T) {
	t.Parallel()
	testMap := map[string]uint64{
		"1 Bps":  byteformats.Byte,
		"1 Kbps": byteformats.KByte / 8,
		"1 KBps": byteformats.KByte,
		"1 Mbps": byteformats.MByte / 8,
		"1 MBps": byteformats.MByte,
		"1 Gbps": byteformats.GByte / 8,
		"1 GBps": byteformats.GByte,
		"1 Tbps": byteformats.TByte / 8,
		"1 TBps": byteformats.TByte,
		"1 Pbps": byteformats.PByte / 8,
		"1 PBps": byteformats.PByte,
		"1k":     byteformats.KByte,
		"1m":     byteformats.MByte,
	}
	for k, v := range testMap {
		var nb byteformats.NetworkBytesCompat
		require.NoError(t, json.Unmarshal([]byte("\""+k+"\""), &nb))
		require.Equal(t, v, nb.Value())
		b, err := json.Marshal(nb)
		require.NoError(t, err)
		require.Equal(t, "\""+k+"\"", string(b))
	}
}

func TestMemoryBytes(t *testing.T) {
	t.Parallel()
	testMap := map[string]uint64{
		"1 B":  byteformats.Byte,
		"1 KB": byteformats.KiByte,
		"1 MB": byteformats.MiByte,
		"1 GB": byteformats.GiByte,
		"1 TB": byteformats.TiByte,
		"1 PB": byteformats.PiByte,
	}
	for k, v := range testMap {
		var mb byteformats.MemoryBytes
		require.NoError(t, json.Unmarshal([]byte("\""+k+"\""), &mb))
		require.Equal(t, v, mb.Value())
		b, err := json.Marshal(mb)
		require.NoError(t, err)
		require.Equal(t, "\""+k+"\"", string(b))
	}
}

func TestDefaultBytes(t *testing.T) {
	t.Parallel()
	testMap := map[string]uint64{
		"1 B":   byteformats.Byte,
		"1 KB":  byteformats.KByte,
		"1 KiB": byteformats.KiByte,
		"1 MB":  byteformats.MByte,
		"1 MiB": byteformats.MiByte,
		"1 GB":  byteformats.GByte,
		"1 GiB": byteformats.GiByte,
		"1 TB":  byteformats.TByte,
		"1 TiB": byteformats.TiByte,
		"1 PB":  byteformats.PByte,
		"1 PiB": byteformats.PiByte,
		"1 EB":  byteformats.EByte,
		"1 EiB": byteformats.EiByte,
		"1k":    byteformats.KByte,
		"1m":    byteformats.MByte,
		"1g":    byteformats.GByte,
		"1t":    byteformats.TByte,
		"1p":    byteformats.PByte,
		"1e":    byteformats.EByte,
		"1K":    byteformats.KByte,
		"1M":    byteformats.MByte,
		"1G":    byteformats.GByte,
		"1T":    byteformats.TByte,
		"1P":    byteformats.PByte,
		"1E":    byteformats.EByte,
		"1Ki":   byteformats.KiByte,
		"1Mi":   byteformats.MiByte,
		"1Gi":   byteformats.GiByte,
		"1Ti":   byteformats.TiByte,
		"1Pi":   byteformats.PiByte,
		"1Ei":   byteformats.EiByte,
		"1KiB":  byteformats.KiByte,
		"1MiB":  byteformats.MiByte,
		"1GiB":  byteformats.GiByte,
		"1TiB":  byteformats.TiByte,
		"1PiB":  byteformats.PiByte,
		"1EiB":  byteformats.EiByte,
		"1kB":   byteformats.KByte,
		"1mB":   byteformats.MByte,
		"1gB":   byteformats.GByte,
		"1tB":   byteformats.TByte,
		"1pB":   byteformats.PByte,
		"1eB":   byteformats.EByte,
	}
	for k, v := range testMap {
		var mb byteformats.Bytes
		require.NoError(t, json.Unmarshal([]byte("\""+k+"\""), &mb))
		require.Equal(t, v, mb.Value())
		b, err := json.Marshal(mb)
		require.NoError(t, err)
		require.Equal(t, "\""+k+"\"", string(b))
	}
}
