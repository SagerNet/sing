package system

import (
	"context"
	"net"
	"net/netip"
	"os"
	"runtime"

	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/buf"
	"github.com/sagernet/sing/common/cache"
	E "github.com/sagernet/sing/common/exceptions"
	"github.com/sagernet/sing/common/log"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
	"github.com/sagernet/sing/common/rw"
	"github.com/sagernet/sing/common/tun"
	"github.com/sagernet/sing/common/udpnat"
	"gvisor.dev/gvisor/pkg/tcpip"
	tcpipBuffer "gvisor.dev/gvisor/pkg/tcpip/buffer"
	"gvisor.dev/gvisor/pkg/tcpip/header"
	"gvisor.dev/gvisor/pkg/tcpip/header/parse"
	"gvisor.dev/gvisor/pkg/tcpip/stack"
)

var logger = log.NewLogger("tun <system>")

type Stack struct {
	tunFd        uintptr
	tunMtu       int
	inetAddress  netip.Prefix
	inet6Address netip.Prefix

	handler tun.Handler

	tunFile      *os.File
	tcpForwarder *net.TCPListener
	tcpPort      uint16
	tcpSessions  *cache.LruCache[netip.AddrPort, netip.AddrPort]
	udpNat       *udpnat.Service[netip.AddrPort]
}

func New(tunFd uintptr, tunMtu int, inetAddress netip.Prefix, inet6Address netip.Prefix, packetTimeout int64, handler tun.Handler) tun.Stack {
	return &Stack{
		tunFd:        tunFd,
		tunMtu:       tunMtu,
		inetAddress:  inetAddress,
		inet6Address: inet6Address,
		handler:      handler,
		tunFile:      os.NewFile(tunFd, "tun"),
		tcpSessions: cache.New(
			cache.WithAge[netip.AddrPort, netip.AddrPort](packetTimeout),
			cache.WithUpdateAgeOnGet[netip.AddrPort, netip.AddrPort](),
		),
		udpNat: udpnat.New[netip.AddrPort](packetTimeout, handler),
	}
}

func (t *Stack) Start() error {
	var network string
	var address net.TCPAddr
	if !t.inet6Address.IsValid() {
		network = "tcp4"
		address.IP = t.inetAddress.Addr().AsSlice()
	} else {
		network = "tcp"
		address.IP = net.IPv6zero
	}

	tcpListener, err := net.ListenTCP(network, &address)
	if err != nil {
		return err
	}

	t.tcpForwarder = tcpListener

	go t.tcpLoop()
	go t.tunLoop()

	return nil
}

func (t *Stack) Close() error {
	t.tcpForwarder.Close()
	t.tunFile.Close()
	return nil
}

func (t *Stack) tunLoop() {
	_buffer := buf.Make(t.tunMtu)
	defer runtime.KeepAlive(_buffer)
	buffer := common.Dup(_buffer)
	for {
		n, err := t.tunFile.Read(buffer)
		if err != nil {
			t.handler.HandleError(err)
			break
		}
		packet := buffer[:n]
		t.deliverPacket(packet)
	}
}

func (t *Stack) deliverPacket(packet []byte) {
	var err error
	switch header.IPVersion(packet) {
	case header.IPv4Version:
		ipHdr := header.IPv4(packet)
		switch ipHdr.TransportProtocol() {
		case header.TCPProtocolNumber:
			err = t.processIPv4TCP(ipHdr, ipHdr.Payload())
		case header.UDPProtocolNumber:
			err = t.processIPv4UDP(ipHdr, ipHdr.Payload())
		default:
			_, err = t.tunFile.Write(packet)
		}
	case header.IPv6Version:
		pkt := stack.NewPacketBuffer(stack.PacketBufferOptions{
			Data: tcpipBuffer.View(packet).ToVectorisedView(),
		})
		proto, _, _, _, ok := parse.IPv6(pkt)
		pkt.DecRef()
		if !ok {
			return
		}
		ipHdr := header.IPv6(packet)
		switch proto {
		case header.TCPProtocolNumber:
			err = t.processIPv6TCP(ipHdr, ipHdr.Payload())
		case header.UDPProtocolNumber:
			err = t.processIPv6UDP(ipHdr, ipHdr.Payload())
		default:
			_, err = t.tunFile.Write(packet)
		}
	}
	if err != nil {
		t.handler.HandleError(err)
	}
}

func (t *Stack) processIPv4TCP(ipHdr header.IPv4, tcpHdr header.TCP) error {
	sourceAddress := ipHdr.SourceAddress()
	destinationAddress := ipHdr.DestinationAddress()
	sourcePort := tcpHdr.SourcePort()
	destinationPort := tcpHdr.DestinationPort()

	logger.Trace(sourceAddress, ":", sourcePort, " => ", destinationAddress, ":", destinationPort)

	if sourcePort != t.tcpPort {
		key := M.AddrPortFrom(net.IP(destinationAddress), sourcePort)
		t.tcpSessions.LoadOrStore(key, func() netip.AddrPort {
			return M.AddrPortFrom(net.IP(sourceAddress), destinationPort)
		})
		ipHdr.SetSourceAddress(destinationAddress)
		ipHdr.SetDestinationAddress(tcpip.Address(t.inetAddress.Addr().AsSlice()))
		tcpHdr.SetDestinationPort(t.tcpPort)
	} else {
		key := M.AddrPortFrom(net.IP(destinationAddress), destinationPort)
		session, loaded := t.tcpSessions.Load(key)
		if !loaded {
			return E.New("unknown tcp session with source port ", destinationPort, " to destination address ", destinationAddress)
		}
		ipHdr.SetSourceAddress(destinationAddress)
		tcpHdr.SetSourcePort(session.Port())
		ipHdr.SetDestinationAddress(tcpip.Address(session.Addr().AsSlice()))
	}

	ipHdr.SetChecksum(0)
	ipHdr.SetChecksum(^ipHdr.CalculateChecksum())
	tcpHdr.SetChecksum(0)
	tcpHdr.SetChecksum(^tcpHdr.CalculateChecksum(header.ChecksumCombine(
		header.PseudoHeaderChecksum(header.TCPProtocolNumber, ipHdr.SourceAddress(), ipHdr.DestinationAddress(), uint16(len(tcpHdr))),
		header.Checksum(tcpHdr.Payload(), 0),
	)))

	_, err := t.tunFile.Write(ipHdr)
	return err
}

func (t *Stack) processIPv6TCP(ipHdr header.IPv6, tcpHdr header.TCP) error {
	sourceAddress := ipHdr.SourceAddress()
	destinationAddress := ipHdr.DestinationAddress()
	sourcePort := tcpHdr.SourcePort()
	destinationPort := tcpHdr.DestinationPort()

	if sourcePort != t.tcpPort {
		key := M.AddrPortFrom(net.IP(destinationAddress), sourcePort)
		t.tcpSessions.LoadOrStore(key, func() netip.AddrPort {
			return M.AddrPortFrom(net.IP(sourceAddress), destinationPort)
		})
		ipHdr.SetSourceAddress(destinationAddress)
		ipHdr.SetDestinationAddress(tcpip.Address(t.inet6Address.Addr().AsSlice()))
		tcpHdr.SetDestinationPort(t.tcpPort)
	} else {
		key := M.AddrPortFrom(net.IP(destinationAddress), destinationPort)
		session, loaded := t.tcpSessions.Load(key)
		if !loaded {
			return E.New("unknown tcp session with source port ", destinationPort, " to destination address ", destinationAddress)
		}
		ipHdr.SetSourceAddress(destinationAddress)
		tcpHdr.SetSourcePort(session.Port())
		ipHdr.SetDestinationAddress(tcpip.Address(session.Addr().AsSlice()))
	}

	tcpHdr.SetChecksum(0)
	tcpHdr.SetChecksum(^tcpHdr.CalculateChecksum(header.ChecksumCombine(
		header.PseudoHeaderChecksum(header.TCPProtocolNumber, ipHdr.SourceAddress(), ipHdr.DestinationAddress(), uint16(len(tcpHdr))),
		header.Checksum(tcpHdr.Payload(), 0),
	)))

	_, err := t.tunFile.Write(ipHdr)
	return err
}

func (t *Stack) tcpLoop() {
	for {
		logger.Trace("tcp start")
		tcpConn, err := t.tcpForwarder.AcceptTCP()
		logger.Trace("tcp accept")
		if err != nil {
			t.handler.HandleError(err)
			return
		}
		key := M.AddrPortFromNet(tcpConn.RemoteAddr())
		session, ok := t.tcpSessions.Load(key)
		if !ok {
			tcpConn.Close()
			logger.Warn("dropped unknown tcp session from ", key)
			continue
		}

		var metadata M.Metadata
		metadata.Protocol = "tun"
		metadata.Source.Addr = session.Addr()
		metadata.Source.Port = key.Port()
		metadata.Destination.Addr = key.Addr()
		metadata.Destination.Port = session.Port()

		go t.processConn(tcpConn, metadata, key)
	}
}

func (t *Stack) processConn(conn *net.TCPConn, metadata M.Metadata, key netip.AddrPort) {
	err := t.handler.NewConnection(context.Background(), conn, metadata)
	if err != nil {
		t.handler.HandleError(err)
	}
	t.tcpSessions.Delete(key)
}

func (t *Stack) processIPv4UDP(ipHdr header.IPv4, hdr header.UDP) error {
	var metadata M.Metadata
	metadata.Protocol = "tun"
	metadata.Source = M.SocksaddrFrom(net.IP(ipHdr.SourceAddress()), hdr.SourcePort())
	metadata.Source = M.SocksaddrFrom(net.IP(ipHdr.DestinationAddress()), hdr.DestinationPort())

	headerCache := buf.New()
	_, err := headerCache.Write(ipHdr[:ipHdr.HeaderLength()+header.UDPMinimumSize])
	if err != nil {
		return err
	}

	logger.Trace("[UDP] ", metadata.Source, "=>", metadata.Destination)

	t.udpNat.NewPacket(context.Background(), metadata.Source.AddrPort(), func() N.PacketWriter {
		return &inetPacketWriter{
			tun:             t,
			headerCache:     headerCache,
			sourceAddress:   ipHdr.SourceAddress(),
			destination:     ipHdr.DestinationAddress(),
			destinationPort: hdr.DestinationPort(),
		}
	}, buf.With(hdr), metadata)
	return nil
}

type inetPacketWriter struct {
	tun             *Stack
	headerCache     *buf.Buffer
	sourceAddress   tcpip.Address
	destination     tcpip.Address
	destinationPort uint16
}

func (w *inetPacketWriter) WritePacket(buffer *buf.Buffer, destination M.Socksaddr) error {
	index := w.headerCache.Len()
	newHeader := w.headerCache.Extend(w.headerCache.Len())
	copy(newHeader, w.headerCache.Bytes())
	w.headerCache.Advance(index)

	defer func() {
		w.headerCache.FullReset()
		w.headerCache.Resize(0, index)
	}()

	var newSourceAddress tcpip.Address
	var newSourcePort uint16

	if destination.IsValid() {
		newSourceAddress = tcpip.Address(destination.Addr.AsSlice())
		newSourcePort = destination.Port
	} else {
		newSourceAddress = w.destination
		newSourcePort = w.destinationPort
	}

	newIpHdr := header.IPv4(newHeader)
	newIpHdr.SetSourceAddress(newSourceAddress)
	newIpHdr.SetTotalLength(uint16(int(w.headerCache.Len()) + buffer.Len()))
	newIpHdr.SetChecksum(0)
	newIpHdr.SetChecksum(^newIpHdr.CalculateChecksum())

	udpHdr := header.UDP(w.headerCache.From(w.headerCache.Len() - header.UDPMinimumSize))
	udpHdr.SetSourcePort(newSourcePort)
	udpHdr.SetLength(uint16(header.UDPMinimumSize + buffer.Len()))
	udpHdr.SetChecksum(0)
	udpHdr.SetChecksum(^udpHdr.CalculateChecksum(header.Checksum(buffer.Bytes(), header.PseudoHeaderChecksum(header.UDPProtocolNumber, newSourceAddress, w.sourceAddress, uint16(header.UDPMinimumSize+buffer.Len())))))

	replyVV := tcpipBuffer.VectorisedView{}
	replyVV.AppendView(newHeader)
	replyVV.AppendView(buffer.Bytes())

	return w.tun.WriteVV(replyVV)
}

func (w *inetPacketWriter) Close() error {
	w.headerCache.Release()
	return nil
}

func (t *Stack) processIPv6UDP(ipHdr header.IPv6, hdr header.UDP) error {
	var metadata M.Metadata
	metadata.Protocol = "tun"
	metadata.Source = M.SocksaddrFrom(net.IP(ipHdr.SourceAddress()), hdr.SourcePort())
	metadata.Destination = M.SocksaddrFrom(net.IP(ipHdr.DestinationAddress()), hdr.DestinationPort())

	headerCache := buf.New()
	_, err := headerCache.Write(ipHdr[:uint16(len(ipHdr))-ipHdr.PayloadLength()+header.UDPMinimumSize])
	if err != nil {
		return err
	}

	t.udpNat.NewPacket(context.Background(), metadata.Source.AddrPort(), func() N.PacketWriter {
		return &inet6PacketWriter{
			tun:             t,
			headerCache:     headerCache,
			sourceAddress:   ipHdr.SourceAddress(),
			destination:     ipHdr.DestinationAddress(),
			destinationPort: hdr.DestinationPort(),
		}
	}, buf.With(hdr), metadata)
	return nil
}

type inet6PacketWriter struct {
	tun             *Stack
	headerCache     *buf.Buffer
	sourceAddress   tcpip.Address
	destination     tcpip.Address
	destinationPort uint16
}

func (w *inet6PacketWriter) WritePacket(buffer *buf.Buffer, destination M.Socksaddr) error {
	index := w.headerCache.Len()
	newHeader := w.headerCache.Extend(w.headerCache.Len())
	copy(newHeader, w.headerCache.Bytes())
	w.headerCache.Advance(index)

	defer func() {
		w.headerCache.FullReset()
		w.headerCache.Resize(0, index)
	}()

	var newSourceAddress tcpip.Address
	var newSourcePort uint16

	if destination.IsValid() {
		newSourceAddress = tcpip.Address(destination.Addr.AsSlice())
		newSourcePort = destination.Port
	} else {
		newSourceAddress = w.destination
		newSourcePort = w.destinationPort
	}

	newIpHdr := header.IPv6(newHeader)
	newIpHdr.SetSourceAddress(newSourceAddress)
	newIpHdr.SetPayloadLength(uint16(header.UDPMinimumSize + buffer.Len()))

	udpHdr := header.UDP(w.headerCache.From(w.headerCache.Len() - header.UDPMinimumSize))
	udpHdr.SetSourcePort(newSourcePort)
	udpHdr.SetLength(uint16(header.UDPMinimumSize + buffer.Len()))
	udpHdr.SetChecksum(0)
	udpHdr.SetChecksum(^udpHdr.CalculateChecksum(header.Checksum(buffer.Bytes(), header.PseudoHeaderChecksum(header.UDPProtocolNumber, newSourceAddress, w.sourceAddress, uint16(header.UDPMinimumSize+buffer.Len())))))

	replyVV := tcpipBuffer.VectorisedView{}
	replyVV.AppendView(newHeader)
	replyVV.AppendView(buffer.Bytes())

	return w.tun.WriteVV(replyVV)
}

func (t *Stack) WriteVV(vv tcpipBuffer.VectorisedView) error {
	data := make([][]byte, 0, len(vv.Views()))
	for _, view := range vv.Views() {
		data = append(data, view)
	}
	return common.Error(rw.WriteV(t.tunFd, data...))
}

func (w *inet6PacketWriter) Close() error {
	w.headerCache.Release()
	return nil
}

type tcpipError struct {
	Err tcpip.Error
}

func (e *tcpipError) Error() string {
	return e.Err.String()
}
