package geosite

import (
	"bytes"
	"io"
	"sort"

	"github.com/sagernet/sing/common"
	E "github.com/sagernet/sing/common/exceptions"
	"github.com/sagernet/sing/common/rw"
)

func Write(writer io.Writer, geosite map[string][]string) error {
	keys := make([]string, 0, len(geosite))
	for code := range geosite {
		keys = append(keys, code)
	}
	sort.Strings(keys)

	content := &bytes.Buffer{}
	index := make(map[string]int)
	for _, code := range keys {
		index[code] = content.Len()
		for _, domain := range geosite[code] {
			if err := rw.WriteVString(content, domain); err != nil {
				return err
			}
		}
	}

	err := rw.WriteByte(writer, 0)
	if err != nil {
		return err
	}

	err = rw.WriteUVariant(writer, uint64(len(keys)))
	if err != nil {
		return err
	}

	for _, code := range keys {
		err = rw.WriteVString(writer, code)
		if err != nil {
			return err
		}
		err = rw.WriteUVariant(writer, uint64(index[code]))
		if err != nil {
			return err
		}
		err = rw.WriteUVariant(writer, uint64(len(geosite[code])))
		if err != nil {
			return err
		}
	}

	_, err = writer.Write(content.Bytes())
	if err != nil {
		return err
	}

	return nil
}

func Read(reader io.Reader, codes ...string) (map[string][]string, error) {
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
	keys := make([]string, length)
	domainIndex := make(map[string]int)
	domainLength := make(map[string]int)
	for i := 0; i < int(length); i++ {
		code, err := rw.ReadVString(reader)
		if err != nil {
			return nil, err
		}
		keys[i] = code
		codeIndex, err := rw.ReadUVariant(reader)
		if err != nil {
			return nil, err
		}
		codeLength, err := rw.ReadUVariant(reader)
		if err != nil {
			return nil, err
		}
		domainIndex[code] = int(codeIndex)
		domainLength[code] = int(codeLength)
	}
	site := make(map[string][]string)
	for _, code := range keys {
		if len(codes) == 0 || common.Contains(codes, code) {
			domains := make([]string, domainLength[code])
			for i := range domains {
				domains[i], err = rw.ReadVString(reader)
				if err != nil {
					return nil, err
				}
			}
			site[code] = domains
		} else {
			dLength := domainLength[code]
			for i := 0; i < dLength; i++ {
				_, err = rw.ReadVString(reader)
				if err != nil {
					return nil, err
				}
			}
		}
	}
	return site, nil
}

func ReadSeek(reader io.ReadSeeker, codes ...string) (map[string][]string, error) {
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
	keys := make([]string, length)
	domainIndex := make(map[string]int)
	domainLength := make(map[string]int)
	for i := 0; i < int(length); i++ {
		code, err := rw.ReadVString(reader)
		if err != nil {
			return nil, err
		}
		keys[i] = code
		codeIndex, err := rw.ReadUVariant(reader)
		if err != nil {
			return nil, err
		}
		codeLength, err := rw.ReadUVariant(reader)
		if err != nil {
			return nil, err
		}
		domainIndex[code] = int(codeIndex)
		domainLength[code] = int(codeLength)
	}
	if len(codes) == 0 {
		codes = keys
	}
	site := make(map[string][]string)
	counter := &rw.ReadCounter{Reader: reader}
	for _, code := range codes {
		domains := make([]string, domainLength[code])
		if _, exists := domainIndex[code]; !exists {
			return nil, E.New("code ", code, " not exists!")
		}
		_, err = reader.Seek(int64(domainIndex[code])-counter.Count(), io.SeekCurrent)
		if err != nil {
			return nil, err
		}
		for i := range domains {
			domains[i], err = rw.ReadVString(reader)
			if err != nil {
				return nil, err
			}
		}
		site[code] = domains
	}

	return site, nil
}

func ReadArray(reader io.ReadSeeker, codes ...string) ([]string, error) {
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
	keys := make([]string, length)
	domainIndex := make(map[string]int)
	domainLength := make(map[string]int)
	for i := 0; i < int(length); i++ {
		code, err := rw.ReadVString(reader)
		if err != nil {
			return nil, err
		}
		keys[i] = code
		codeIndex, err := rw.ReadUVariant(reader)
		if err != nil {
			return nil, err
		}
		codeLength, err := rw.ReadUVariant(reader)
		if err != nil {
			return nil, err
		}
		domainIndex[code] = int(codeIndex)
		domainLength[code] = int(codeLength)
	}
	if len(codes) == 0 {
		codes = keys
	}
	var domainL int
	for _, code := range keys {
		if _, exists := domainIndex[code]; !exists {
			return nil, E.New("code ", code, " not exists!")
		}
		domainL += domainLength[code]
	}
	domains := make([]string, 0, domainL)
	counter := &rw.ReadCounter{Reader: reader}
	for _, code := range codes {
		_, err := reader.Seek(int64(domainIndex[code])-counter.Count(), io.SeekCurrent)
		if err != nil {
			return nil, err
		}
		codeL := domainLength[code]
		for i := 0; i < codeL; i++ {
			domain, err := rw.ReadVString(reader)
			if err != nil {
				return nil, err
			}
			domains = append(domains, domain)
		}
	}

	return domains, nil
}
