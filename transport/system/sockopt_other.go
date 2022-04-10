//go:build !linux

package main

import "github.com/sagernet/sing/common/exceptions"

func TCPFastOpen(fd uintptr) error {
	return exceptions.New("only available on linux")
}

func TProxy(fd uintptr, isIPv6 bool) error {
	return exceptions.New("only available on linux")
}
