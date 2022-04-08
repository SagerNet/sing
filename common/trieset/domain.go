package trieset

import (
	"bytes"
	"errors"
	"sort"
	"strings"

	"github.com/samber/lo"
)

// ErrInvalidDomain means insert domain is invalid
var ErrInvalidDomain = errors.New("invalid domain")

func reverse(s string) []byte {
	bytes := []byte(s)
	for i2, j := 0, len(bytes)-1; i2 < j; i2, j = i2+1, j-1 {
		bytes[i2], bytes[j] = bytes[j], bytes[i2]
	}
	return bytes
}

type DomainSet struct {
	set *Set
}

// Has query for a key and return whether it presents in the Set.
func (ds *DomainSet) Has(domain string) bool {
	return ds.has(reverse(domain), 0, 0)
}

func (ds *DomainSet) has(key []byte, nodeId, bmIdx int) bool {
	for i := 0; i < len(key); i++ {
		c := key[i]
	Outer:
		for ; ; bmIdx++ {
			if getBit(ds.set.labelBitmap, bmIdx) != 0 {
				// no more labels in this node
				return false
			}

			switch char := ds.set.labels[bmIdx-nodeId]; char {
			case '.':
				nodeId := countZeros(ds.set.labelBitmap, ds.set.ranks, bmIdx+1)
				if getBit(ds.set.leaves, nodeId) != 0 && c == '.' {
					return true
				} else if char == c {
					break Outer
				}
			case c:
				break Outer
			case '*':
				idx := bytes.IndexByte(key[i:], '.')
				nodeId := countZeros(ds.set.labelBitmap, ds.set.ranks, bmIdx+1)
				if idx == -1 {
					return getBit(ds.set.leaves, nodeId) != 0
				}

				bmIdx := selectIthOne(ds.set.labelBitmap, ds.set.ranks, ds.set.selects, nodeId-1) + 1
				if ds.has(key[i+idx:], nodeId, bmIdx) {
					return true
				}
			}
		}

		// go to next level
		nodeId = countZeros(ds.set.labelBitmap, ds.set.ranks, bmIdx+1)
		bmIdx = selectIthOne(ds.set.labelBitmap, ds.set.ranks, ds.set.selects, nodeId-1) + 1
	}

	return getBit(ds.set.leaves, nodeId) != 0
}

func New(domains []string) (*DomainSet, error) {
	list := make([]string, len(domains))

	for i, domain := range domains {
		if domain == "" || domain[len(domain)-1] == '.' {
			return nil, ErrInvalidDomain
		}

		domain = string(reverse(domain))
		if strings.HasSuffix(domain, "+") {
			list[i] = domain[:len(domain)-1]
			list = append(list, domain[:len(domain)-2])
		} else {
			list[i] = domain
		}
	}

	sort.Strings(list)
	list = lo.Uniq(list)

	return &DomainSet{NewSet(list)}, nil
}
