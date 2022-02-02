package common

import "io"

type Flusher interface {
	Flush() error
}

func Flush(writer io.Writer) error {
	for {
		if f, ok := writer.(Flusher); ok {
			err := f.Flush()
			if err != nil {
				return err
			}
		}
		if u, ok := writer.(WriterWithUpstream); ok {
			writer = u.Upstream()
		} else {
			break
		}
	}
	return nil
}
