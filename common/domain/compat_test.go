package domain

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/sagernet/sing/common/varbin"

	"github.com/stretchr/testify/require"
)

func oldWriteUint64Slice(writer varbin.Writer, value []uint64) error {
	_, err := varbin.WriteUvarint(writer, uint64(len(value)))
	if err != nil {
		return err
	}
	if len(value) == 0 {
		return nil
	}
	return binary.Write(writer, binary.BigEndian, value)
}

func oldWriteByteSlice(writer varbin.Writer, value []byte) error {
	_, err := varbin.WriteUvarint(writer, uint64(len(value)))
	if err != nil {
		return err
	}
	if len(value) == 0 {
		return nil
	}
	return binary.Write(writer, binary.BigEndian, value)
}

func oldWriteSuccinctSet(writer varbin.Writer, set *succinctSet) error {
	err := writer.WriteByte(0)
	if err != nil {
		return err
	}
	err = oldWriteUint64Slice(writer, set.leaves)
	if err != nil {
		return err
	}
	err = oldWriteUint64Slice(writer, set.labelBitmap)
	if err != nil {
		return err
	}
	return oldWriteByteSlice(writer, set.labels)
}

func TestMatcherSerializationCompat(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		domains     []string
		suffixes    []string
		legacy      bool
		queryInputs []string
	}{
		{
			name:     "standard",
			domains:  []string{"example.com", "example.org", "nested.example.com"},
			suffixes: []string{".com.cn", "sagernet.org", ".example.net"},
			legacy:   false,
			queryInputs: []string{
				"example.com",
				"sub.example.com",
				"example.org",
				"example.com.cn",
				"test.example.com.cn",
				"sagernet.org",
				"sub.sagernet.org",
				"unknown.example",
			},
		},
		{
			name:     "legacy",
			domains:  []string{"alpha.test", "beta.test"},
			suffixes: []string{".legacy.example", "legacy.example"},
			legacy:   true,
			queryInputs: []string{
				"alpha.test",
				"beta.test",
				"gamma.test",
				"sub.legacy.example",
				"legacy.example",
				"example.legacy",
			},
		},
		{
			name:     "mixed",
			domains:  []string{"a.b", "b.a", "dash-name.example"},
			suffixes: []string{".co.uk", "example.com"},
			legacy:   false,
			queryInputs: []string{
				"a.b",
				"b.a",
				"dash-name.example",
				"sub.dash-name.example",
				"example.com",
				"sub.example.com",
				"example.co.uk",
			},
		},
	}

	for _, testCase := range cases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			matcher := NewMatcher(testCase.domains, testCase.suffixes, testCase.legacy)
			require.NotNil(t, matcher)

			var oldBuffer bytes.Buffer
			err := oldWriteSuccinctSet(&oldBuffer, matcher.set)
			require.NoError(t, err)

			var newBuffer bytes.Buffer
			err = matcher.Write(&newBuffer)
			require.NoError(t, err)

			require.Equal(t, oldBuffer.Bytes(), newBuffer.Bytes())

			readMatcher, err := ReadMatcher(bytes.NewReader(oldBuffer.Bytes()))
			require.NoError(t, err)

			domains, suffixes := matcher.Dump()
			readDomains, readSuffixes := readMatcher.Dump()
			require.Equal(t, domains, readDomains)
			require.Equal(t, suffixes, readSuffixes)

			for _, queryInput := range testCase.queryInputs {
				require.Equal(t, matcher.Match(queryInput), readMatcher.Match(queryInput), queryInput)
			}
		})
	}
}

func TestAdGuardMatcherSerializationCompat(t *testing.T) {
	t.Parallel()

	rules := []string{
		"||ads.example.com^",
		"||tracker.example.org^",
		"|exact.example.net^",
		"example.com",
		"||*.wild.example^",
		"||suffix.example",
	}
	queryInputs := []string{
		"ads.example.com",
		"sub.ads.example.com",
		"tracker.example.org",
		"exact.example.net",
		"prefix.example.net",
		"example.com",
		"sub.example.com",
		"wild.example",
		"sub.wild.example",
		"suffix.example",
		"sub.suffix.example",
		"unmatched.example",
	}

	matcher := NewAdGuardMatcher(rules)
	require.NotNil(t, matcher)

	var oldBuffer bytes.Buffer
	err := oldWriteSuccinctSet(&oldBuffer, matcher.set)
	require.NoError(t, err)

	var newBuffer bytes.Buffer
	err = matcher.Write(&newBuffer)
	require.NoError(t, err)

	require.Equal(t, oldBuffer.Bytes(), newBuffer.Bytes())

	readMatcher, err := ReadAdGuardMatcher(bytes.NewReader(oldBuffer.Bytes()))
	require.NoError(t, err)

	for _, queryInput := range queryInputs {
		require.Equal(t, matcher.Match(queryInput), readMatcher.Match(queryInput), queryInput)
	}
}
