package socks4

import "context"

type UserContext struct {
	context.Context
	Username string
}

func (c *UserContext) Upstream() any {
	return c.Context
}
