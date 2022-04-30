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
			if u.WriterReplaceable() {
				if writerBack == writer {
				} else if setter, hasSetter := u.UpstreamWriter().(UpstreamWriterSetter); hasSetter {
					setter.SetWriter(writerBack)
					writer = u.UpstreamWriter()
					continue
				}
			}
			writerBack = writer
			writer = u.UpstreamWriter()
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
			if u.WriterReplaceable() {
				if writerBack == writer {
					writer = u.UpstreamWriter()
					writerBack = writer
					*writerP = writer
					continue
				} else if setter, hasSetter := writerBack.(UpstreamWriterSetter); hasSetter {
					setter.SetWriter(u.UpstreamWriter())
					writer = u.UpstreamWriter()
					continue
				}
			}
			writerBack = writer
			writer = u.UpstreamWriter()
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

func (w *FlushOnceWriter) UpstreamWriter() io.Writer {
	return w.Writer
}

func (w *FlushOnceWriter) WriterReplaceable() bool {
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
