package main

import "github.com/sagernet/sing/common/acme"

type Config struct {
	URL   string         `json:"url"`
	Token string         `json:"token"`
	Nodes []Node         `json:"nodes"`
	Debug bool           `json:"debug"`
	ACME  *acme.Settings `json:"acme"`
}

type Node struct {
	ID     int    `json:"id"`
	Type   string `json:"type"`
	Domain string `json:"domain"`
}
