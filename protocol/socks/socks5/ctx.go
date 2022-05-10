package socks5

import "context"

type UserContext struct {
	context.Context
	Username string
	Password string
}
