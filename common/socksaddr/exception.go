package socksaddr

import "fmt"

type StringTooLongException struct {
	Op  string
	Len int
}

func (e StringTooLongException) Error() string {
	return fmt.Sprint(e.Op, " too long: length ", e.Len, ", max 255")
}
