package common

type Comparable[T any] interface {
	Equals(other T) bool
}

func Equals[T Comparable[T]](obj T, other T) bool {
	var anyObj any = obj
	var anyOther any = other
	if anyObj == nil && anyOther == nil {
		return true
	} else if anyObj == nil || anyOther == nil {
		return false
	}
	return obj.Equals(other)
}

func IsEmptyByEquals[T Comparable[T]](obj T) bool {
	return obj.Equals(DefaultValue[T]())
}

func ComparablePtrEquals[T comparable](obj *T, other *T) bool {
	return *obj == *other
}

func PtrEquals[T Comparable[T]](obj *T, other *T) bool {
	return Equals(*obj, *other)
}

func ComparableSliceEquals[T comparable](arr []T, otherArr []T) bool {
	return len(arr) == len(otherArr) && AllIndexed(arr, func(index int, it T) bool {
		return it == otherArr[index]
	})
}

func SliceEquals[T Comparable[T]](arr []T, otherArr []T) bool {
	return len(arr) == len(otherArr) && AllIndexed(arr, func(index int, it T) bool {
		return Equals(it, otherArr[index])
	})
}
