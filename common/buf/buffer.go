package buf

var Empty []byte

func init() {
	Empty = make([]byte, 128)
}

func ForeachN(b []byte, size int) [][]byte {
	total := len(b)
	var index int
	var retArr [][]byte
	for {
		nextIndex := index + size
		if nextIndex < total {
			retArr = append(retArr, b[index:nextIndex])
			index = nextIndex
		} else {
			retArr = append(retArr, b[index:])
			break
		}
	}
	return retArr
}
