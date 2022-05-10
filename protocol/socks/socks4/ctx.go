package socks4

import "context"

type UserContext struct {
	context.Context
	Username string
}
