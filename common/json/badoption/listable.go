package badoption

import (
	E "github.com/sagernet/sing/common/exceptions"
	"github.com/sagernet/sing/common/json"
)

type Listable[T any] []T

func (l Listable[T]) MarshalJSON() ([]byte, error) {
	arrayList := []T(l)
	if len(arrayList) == 1 {
		return json.Marshal(arrayList[0])
	}
	return json.Marshal(arrayList)
}

func (l *Listable[T]) UnmarshalJSON(content []byte) error {
	err := json.UnmarshalDisallowUnknownFields(content, (*[]T)(l))
	if err == nil {
		return nil
	}
	var singleItem T
	newError := json.UnmarshalDisallowUnknownFields(content, &singleItem)
	if newError != nil {
		return E.Errors(err, newError)
	}
	*l = []T{singleItem}
	return nil
}
