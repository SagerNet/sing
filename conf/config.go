package conf

import (
	"encoding/json"

	"sing/common/exceptions"
	"sing/core"
	"sing/transport"
	"sing/transport/block"
	"sing/transport/socks"
	"sing/transport/system"
)

type Config struct {
	Inbounds  []*InboundConfig  `json:"inbounds,omitempty"`
	Outbounds []*OutboundConfig `json:"outbounds,omitempty"`
}

type InboundConfig struct {
	Type     string          `json:"type"`
	Tag      string          `json:"tag,omitempty"`
	Settings json.RawMessage `json:"settings,omitempty"`
}

func (c InboundConfig) Build(instance core.Instance) (transport.Inbound, error) {
	switch c.Type {
	case "socks":
		config := new(socks.InboundConfig)
		err := json.Unmarshal(c.Settings, config)
		if err != nil {
			return nil, err
		}
		return socks.NewListener(instance, config)
	}
	return nil, exceptions.New("unknown inbound type ", c.Type)
}

type OutboundConfig struct {
	Type     string          `json:"type"`
	Settings json.RawMessage `json:"settings,omitempty"`
}

func (c OutboundConfig) Build(instance core.Instance) (transport.Outbound, error) {
	var outbound transport.Outbound
	switch c.Type {
	case "system":
		outbound = new(system.Outbound)
	case "block":
		outbound = new(block.Outbound)
	default:
		return nil, exceptions.New("unknown outbound type: ", c.Type)
	}
	return outbound, nil
}
