package geosite

import (
	"io"
	"strings"

	E "github.com/sagernet/sing/common/exceptions"
	"github.com/sagernet/sing/common/rw"
)

type Reader struct {
	reader       io.ReadSeeker
	counter      *rw.ReadCounter
	domainIndex  map[string]int
	domainLength map[string]int
	cache        map[string][]string
}

func NewReader(reader io.ReadSeeker) (*Reader, error) {
	r := &Reader{
		reader:       reader,
		counter:      &rw.ReadCounter{Reader: reader},
		domainIndex:  map[string]int{},
		domainLength: map[string]int{},
		cache:        map[string][]string{},
	}

	version, err := rw.ReadByte(reader)
	if err != nil {
		return nil, err
	}
	if version != 0 {
		return nil, E.New("bad version")
	}
	length, err := rw.ReadUVariant(reader)
	if err != nil {
		return nil, err
	}

	for i := 0; i < int(length); i++ {
		code, err := rw.ReadVString(reader)
		if err != nil {
			return nil, err
		}
		domainIndex, err := rw.ReadUVariant(reader)
		if err != nil {
			return nil, err
		}
		domainLength, err := rw.ReadUVariant(reader)
		if err != nil {
			return nil, err
		}
		r.domainIndex[code] = int(domainIndex)
		r.domainLength[code] = int(domainLength)
	}

	return r, nil
}

func (r *Reader) Load(code string) ([]string, error) {
	code = strings.ToLower(code)
	if cache, ok := r.cache[code]; ok {
		return cache, nil
	}
	index, exists := r.domainIndex[code]
	if !exists {
		return nil, E.New("code ", code, " not exists!")
	}
	_, err := r.reader.Seek(int64(index)-r.counter.Count(), io.SeekCurrent)
	if err != nil {
		return nil, err
	}
	r.counter.Reset()
	dLength := r.domainLength[code]
	domains := make([]string, dLength)
	for i := 0; i < dLength; i++ {
		domains[i], err = rw.ReadVString(r.counter)
		if err != nil {
			return nil, err
		}
	}
	r.cache[code] = domains
	return domains, nil
}
