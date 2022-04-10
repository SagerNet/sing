package redir

type TransproxyMode uint8

const (
	ModeDisabled TransproxyMode = iota
	ModeRedirect
	ModeTProxy
)
