package common

import (
	"io"
)

type Flusher interface {
	Flush() error
}

func Flush(writer io.Writer) error {
	writerBack := writer
	for {
		if f, ok := writer.(Flusher); ok {
			err := f.Flush()
			if err != nil {
				return err
			}
		}
		if u, ok := writer.(WriterWithUpstream); ok {
			if u.Replaceable() {
				if writerBack == writer {
				} else if setter, hasSetter := u.Upstream().(UpstreamWriterSetter); hasSetter {
					setter.SetWriter(writerBack)
					writer = u.Upstream()
					continue
				}
			}
			writerBack = writer
			writer = u.Upstream()
		} else {
			break
		}
	}
	return nil
}

func FlushVar(writerP *io.Writer) error {
	writer := *writerP
	writerBack := writer
	for {
		if f, ok := writer.(Flusher); ok {
			err := f.Flush()
			if err != nil {
				return err
			}
		}
		if u, ok := writer.(WriterWithUpstream); ok {
			if u.Replaceable() {
				if writerBack == writer {
					writer = u.Upstream()
					writerBack = writer
					*writerP = writer
					continue
				} else if setter, hasSetter := writerBack.(UpstreamWriterSetter); hasSetter {
					setter.SetWriter(u.Upstream())
					writer = u.Upstream()
					continue
				}
			}
			writerBack = writer
			writer = u.Upstream()
		} else {
			break
		}
	}
	return nil
}

type FlushOnceWriter struct {
	io.Writer
	flushed bool
}

func (w *FlushOnceWriter) Upstream() io.Writer {
	return w.Writer
}

func (w *FlushOnceWriter) Replaceable() bool {
	return w.flushed
}

func (w *FlushOnceWriter) Write(p []byte) (n int, err error) {
	if w.flushed {
		return w.Writer.Write(p)
	}
	n, err = w.Writer.Write(p)
	if n > 0 {
		err = FlushVar(&w.Writer)
	}
	if err == nil {
		w.flushed = true
	}
	return
}
