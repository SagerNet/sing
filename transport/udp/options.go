package udp

import "github.com/sagernet/sing/common/redir"

type Option func(*Listener)

func WithTransproxyMode(mode redir.TransproxyMode) Option {
	return func(listener *Listener) {
		listener.tproxy = mode == redir.ModeTProxy
	}
}
