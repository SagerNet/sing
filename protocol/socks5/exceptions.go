package socks5

import "fmt"

type UnsupportedVersionException struct {
	Version byte
}

func (e UnsupportedVersionException) Error() string {
	return fmt.Sprint("unsupported version: ", e.Version)
}

type UnsupportedCommandException struct {
	Command byte
}

func (e UnsupportedCommandException) Error() string {
	return fmt.Sprint("unsupported command: ", e.Command)
}

type UsernamePasswordAuthFailureException struct{}

func (e UsernamePasswordAuthFailureException) Error() string {
	return "username/password auth failed"
}
