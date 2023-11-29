package domain

import (
	"encoding/binary"
	"io"
	"sort"
	"unicode/utf8"

	"github.com/sagernet/sing/common/rw"
)

type Matcher struct {
	set *succinctSet
}

func NewMatcher(domains []string, domainSuffix []string) *Matcher {
	domainList := make([]string, 0, len(domains)+len(domainSuffix))
	seen := make(map[string]bool, len(domainList))
	for _, domain := range domainSuffix {
		if seen[domain] {
			continue
		}
		seen[domain] = true
		domainList = append(domainList, reverseDomainSuffix(domain))
	}
	for _, domain := range domains {
		if seen[domain] {
			continue
		}
		seen[domain] = true
		domainList = append(domainList, reverseDomain(domain))
	}
	sort.Strings(domainList)
	return &Matcher{newSuccinctSet(domainList)}
}

func ReadMatcher(reader io.Reader) (*Matcher, error) {
	var version uint8
	err := binary.Read(reader, binary.BigEndian, &version)
	if err != nil {
		return nil, err
	}
	leavesLength, err := rw.ReadUVariant(reader)
	if err != nil {
		return nil, err
	}
	leaves := make([]uint64, leavesLength)
	err = binary.Read(reader, binary.BigEndian, leaves)
	if err != nil {
		return nil, err
	}
	labelBitmapLength, err := rw.ReadUVariant(reader)
	if err != nil {
		return nil, err
	}
	labelBitmap := make([]uint64, labelBitmapLength)
	err = binary.Read(reader, binary.BigEndian, labelBitmap)
	if err != nil {
		return nil, err
	}
	labelsLength, err := rw.ReadUVariant(reader)
	if err != nil {
		return nil, err
	}
	labels := make([]byte, labelsLength)
	_, err = io.ReadFull(reader, labels)
	if err != nil {
		return nil, err
	}
	set := &succinctSet{
		leaves:      leaves,
		labelBitmap: labelBitmap,
		labels:      labels,
	}
	set.init()
	return &Matcher{set}, nil
}

func (m *Matcher) Match(domain string) bool {
	return m.set.Has(reverseDomain(domain))
}

func (m *Matcher) Write(writer io.Writer) error {
	err := binary.Write(writer, binary.BigEndian, byte(1))
	if err != nil {
		return err
	}
	err = rw.WriteUVariant(writer, uint64(len(m.set.leaves)))
	if err != nil {
		return err
	}
	err = binary.Write(writer, binary.BigEndian, m.set.leaves)
	if err != nil {
		return err
	}
	err = rw.WriteUVariant(writer, uint64(len(m.set.labelBitmap)))
	if err != nil {
		return err
	}
	err = binary.Write(writer, binary.BigEndian, m.set.labelBitmap)
	if err != nil {
		return err
	}
	err = rw.WriteUVariant(writer, uint64(len(m.set.labels)))
	if err != nil {
		return err
	}
	_, err = writer.Write(m.set.labels)
	if err != nil {
		return err
	}
	return nil
}

func reverseDomain(domain string) string {
	l := len(domain)
	b := make([]byte, l)
	for i := 0; i < l; {
		r, n := utf8.DecodeRuneInString(domain[i:])
		i += n
		utf8.EncodeRune(b[l-i:], r)
	}
	return string(b)
}

func reverseDomainSuffix(domain string) string {
	l := len(domain)
	b := make([]byte, l+1)
	for i := 0; i < l; {
		r, n := utf8.DecodeRuneInString(domain[i:])
		i += n
		utf8.EncodeRune(b[l-i:], r)
	}
	b[l] = prefixLabel
	return string(b)
}
