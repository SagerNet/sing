package wapd

var OptionCode = optionCode{}

type optionCode struct{}

func (C optionCode) Code() uint8 {
	return 252
}

func (C optionCode) String() string {
	return "Web Proxy Auto-Discovery Protocol"
}
