package exceptions

import (
    "errors"

    "github.com/sagernet/quic-go"
)

func isCanceledQuic(err error) bool {
    var se *quic.StreamError
    return errors.As(err, &se)
}
