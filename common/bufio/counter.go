package bufio

import (
	"io"

	N "github.com/sagernet/sing/common/network"
)

type CountFunc func(n int64)

type ReadCounter interface {
	io.Reader
	UnwrapReader() (io.Reader, []CountFunc)
}

type WriteCounter interface {
	io.Writer
	UnwrapWriter() (io.Writer, []CountFunc)
}

func UnwrapCountReader(reader io.Reader) (io.Reader, []CountFunc) {
	return unwrapCountReader(reader, nil)
}

func unwrapCountReader(reader io.Reader, countFunc []CountFunc) (io.Reader, []CountFunc) {
	reader = N.UnwrapReader(reader)
	if counter, isCounter := reader.(ReadCounter); isCounter {
		upstreamReader, upstreamCountFunc := counter.UnwrapReader()
		countFunc = append(countFunc, upstreamCountFunc...)
		return unwrapCountReader(upstreamReader, countFunc)
	}
	return reader, countFunc
}

func UnwrapCountWriter(writer io.Writer) (io.Writer, []CountFunc) {
	return unwrapCountWriter(writer, nil)
}

func unwrapCountWriter(writer io.Writer, countFunc []CountFunc) (io.Writer, []CountFunc) {
	writer = N.UnwrapWriter(writer)
	if counter, isCounter := writer.(WriteCounter); isCounter {
		upstreamWriter, upstreamCountFunc := counter.UnwrapWriter()
		countFunc = append(countFunc, upstreamCountFunc...)
		return unwrapCountWriter(upstreamWriter, countFunc)
	}
	return writer, countFunc
}
