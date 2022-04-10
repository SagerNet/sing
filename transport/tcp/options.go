package tcp

import "github.com/sagernet/sing/common/redir"

type Option func(*Listener)

func WithTransproxyMode(mode redir.TransproxyMode) Option {
	return func(listener *Listener) {
		listener.trans = mode
	}
}
