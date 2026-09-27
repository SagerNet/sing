//go:build !(linux || windows || darwin)

package control

func DisableUDPFragment() Func {
	return nil
}

func EnableUDPFragment() Func {
	return nil
}
