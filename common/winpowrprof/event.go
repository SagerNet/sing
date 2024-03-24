package winpowrprof

const (
	EVENT_SUSPEND = iota
	EVENT_RESUME
	EVENT_RESUME_AUTOMATIC // Because the user is not present, most applications should do nothing.
)

type EventListener interface {
	Start() error
	Close() error
}
