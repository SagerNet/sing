package geosite

import (
	"bufio"
	"encoding/binary"
	"github.com/klauspost/compress/zstd"
	"github.com/sagernet/sing/common/exceptions"
	"github.com/sagernet/sing/common/rw"
	"github.com/sagernet/sing/common/trieset"
	"io"
	"strings"
)

type Matcher struct {
	ds *trieset.DomainSet
}

func (m *Matcher) Match(domain string) bool {
	return m.ds.Has(domain)
}

func LoadGeositeMatcher(reader io.Reader, code string) (*Matcher, error) {
	version, err := rw.ReadByte(reader)
	if err != nil {
		return nil, err
	}
	if version != 0 {
		return nil, exceptions.New("bad geosite data")
	}
	decoder, err := zstd.NewReader(reader, zstd.WithDecoderLowmem(true), zstd.WithDecoderConcurrency(1))
	if err != nil {
		return nil, err
	}
	defer decoder.Close()
	bufferedReader := bufio.NewReader(decoder)
	geositeLength, err := binary.ReadUvarint(bufferedReader)
	if err != nil {
		return nil, err
	}
	for geositeLength > 0 {
		geositeLength--
		countryCode, err := rw.ReadVString(bufferedReader)
		if err != nil {
			return nil, err
		}
		domainLength, err := binary.ReadUvarint(bufferedReader)
		if err != nil {
			return nil, err
		}
		if strings.EqualFold(code, countryCode) {
			domains := make([]string, 0, domainLength)
			for domainLength > 0 {
				domainLength--
				domain, err := rw.ReadVString(bufferedReader)
				if err != nil {
					return nil, err
				}
				domains = append(domains, domain)
			}
			ds, err := trieset.New(domains)
			if err != nil {
				return nil, err
			}
			return &Matcher{ds}, nil
		} else {
			for domainLength > 0 {
				domainLength--
				_, err = rw.ReadVString(bufferedReader)
				if err != nil {
					return nil, err
				}
			}
		}
	}
	return nil, exceptions.New(code, " not found in geosite")
}
