package rw

import (
	"context"
	"io"
	"net"

	"sing/common"
	"sing/common/buf"
	"sing/common/task"
)

func CopyConn(ctx context.Context, conn net.Conn, outConn net.Conn) error {
	return task.Run(ctx, func() error {
		return common.Error(io.Copy(conn, outConn))
	}, func() error {
		return common.Error(io.Copy(outConn, conn))
	})
}

func CopyPacketConn(ctx context.Context, conn net.PacketConn, outPacketConn net.PacketConn) error {
	return task.Run(ctx, func() error {
		buffer := buf.FullNew()
		defer buffer.Release()
		for {
			n, addr, err := conn.ReadFrom(buffer.FreeBytes())
			if err != nil {
				return err
			}
			buffer.Truncate(n)
			_, err = outPacketConn.WriteTo(buffer.Bytes(), addr)
			if err != nil {
				return err
			}
			buffer.FullReset()
		}
	}, func() error {
		buffer := buf.FullNew()
		defer buffer.Release()
		for {
			n, addr, err := outPacketConn.ReadFrom(buffer.FreeBytes())
			if err != nil {
				return err
			}
			buffer.Truncate(n)
			_, err = conn.WriteTo(buffer.Bytes(), addr)
			if err != nil {
				return err
			}
			buffer.FullReset()
		}
	})
}
