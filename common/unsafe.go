package common

import "unsafe"

func PointerOf(reference any) uintptr {
	return (uintptr)(unsafe.Pointer(reference.(*interface{})))
}

func PointerEquals(reference any, other any) bool {
	return PointerOf(reference) == PointerOf(other)
}
