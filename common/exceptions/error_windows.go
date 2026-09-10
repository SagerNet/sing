package exceptions

import "golang.org/x/sys/windows"

func init() {
	closedErrors = append(closedErrors,
		windows.WSAECONNABORTED,
		windows.WSAECONNRESET,
		windows.WSAENOTCONN,
	)
}
