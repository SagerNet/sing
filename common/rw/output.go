package rw

import (
	"io"

	"sing/common"
	"sing/common/buf"
	"sing/common/list"
)

type OutputStream interface {
	common.WriterWithUpstream
	Process(p []byte) (n int, buffer *buf.Buffer, flush bool, err error)
}

type DirectException struct {
	Suppressed error
}

func (e *DirectException) Error() string {
	return "upstream used directly"
}

type processFunc func(p []byte) (n int, buffer *buf.Buffer, flush bool, err error)

type OutputStreamWriter struct {
	upstream io.Writer
	chain    list.List[processFunc]
}

func (w *OutputStreamWriter) Upstream() io.Writer {
	return w.upstream
}

func (w *OutputStreamWriter) Write(p []byte) (n int, err error) {
	var needFlush bool
	var buffers list.List[*buf.Buffer]
	defer buf.ReleaseMulti(&buffers)

	for stream := w.chain.Back(); stream != nil; stream = stream.Prev() {
		// TODO: remove cast
		var process processFunc = stream.Value
		processed, buffer, flush, err := process(p)
		if buffer != nil {
			p = buffer.Bytes()
			processed = buffer.Len()
			buffers.PushBack(buffer)
		}
		if err != nil {
			if directException, isDirectException := err.(*DirectException); isDirectException {
				return processed, directException.Suppressed
			}
			return 0, err
		}
		p = p[:processed]
		if flush {
			needFlush = true
		}
	}
	n, err = w.upstream.Write(p)
	if err != nil {
		return
	}

	if needFlush {
		err = common.Flush(w.upstream)
	}

	return
}

func GetWriter(writer io.Writer) io.Writer {
	if _, isOutputStreamWriter := writer.(*OutputStreamWriter); isOutputStreamWriter {
		return writer
	}

	output := OutputStreamWriter{}
	for index := 0; ; index++ {
		if outputStream, isOutputStream := writer.(OutputStream); isOutputStream {
			output.chain.PushFront(outputStream.Process)
			writer = outputStream.Upstream()
		} else if outputStreamWriter, isOutputStreamWriter := writer.(*OutputStreamWriter); isOutputStreamWriter {
			writer = outputStreamWriter.upstream
			output.chain.PushFrontList(&outputStreamWriter.chain)
		} else {
			if index == 0 {
				return writer
			}
			break
		}
	}
	output.upstream = writer
	return &output
}
