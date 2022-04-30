package main

import "encoding/json"

type Config struct {
	URL   string     `json:"url"`
	Token string     `json:"token"`
	Nodes []Node     `json:"nodes"`
	TLS   *TLSConfig `json:"tls,omitempty"`
	Debug bool       `json:"debug"`
}

type Node struct {
	ID     int    `json:"id"`
	Type   string `json:"type"`
	Domain string `json:"domain"`
}

type TLSConfig struct {
	Insecure    bool            `json:"insecure"`
	Email       string          `json:"email"`
	DNSProvider string          `json:"dns_provider"`
	DNSEnv      json.RawMessage `json:"dns_env"`
}
