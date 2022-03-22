package common

const EmptyString = ""

type DummyAddr struct{}

func (d *DummyAddr) Network() string {
	return "dummy"
}

func (d *DummyAddr) String() string {
	return "dummy"
}
