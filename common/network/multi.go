package network

import (
	"context"
	"net"
	"net/netip"
	"time"

	"github.com/sagernet/sing/common"
	E "github.com/sagernet/sing/common/exceptions"
	M "github.com/sagernet/sing/common/metadata"
)

const DefaultFallbackDelay = 300 * time.Millisecond

func DialSerial(ctx context.Context, dialer Dialer, network string, destination M.Socksaddr, destinationAddresses []netip.Addr) (net.Conn, error) {
	if parallelDialer, isParallel := dialer.(ParallelDialer); isParallel {
		return parallelDialer.DialParallel(ctx, network, destination, destinationAddresses)
	}
	var conn net.Conn
	var err error
	var connErrors []error
	for _, address := range destinationAddresses {
		conn, err = dialer.DialContext(ctx, network, M.SocksaddrFrom(address, destination.Port))
		if err != nil {
			connErrors = append(connErrors, err)
			continue
		}
		return conn, nil
	}
	return nil, E.Errors(connErrors...)
}

func ListenSerial(ctx context.Context, dialer Dialer, destination M.Socksaddr, destinationAddresses []netip.Addr) (net.PacketConn, netip.Addr, error) {
	var conn net.PacketConn
	var err error
	var connErrors []error
	for _, address := range destinationAddresses {
		conn, err = dialer.ListenPacket(ctx, M.SocksaddrFrom(address, destination.Port))
		if err != nil {
			connErrors = append(connErrors, err)
			continue
		}
		return conn, address, nil
	}
	return nil, netip.Addr{}, E.Errors(connErrors...)
}

func DialParallel(ctx context.Context, dialer Dialer, network string, destination M.Socksaddr, destinationAddresses []netip.Addr, preferIPv6 bool, fallbackDelay time.Duration) (net.Conn, error) {
	if len(destinationAddresses) == 1 {
		return dialer.DialContext(ctx, network, M.SocksaddrFrom(destinationAddresses[0], destination.Port))
	}
	if fallbackDelay == 0 {
		fallbackDelay = DefaultFallbackDelay
	}
	addresses4 := common.Filter(destinationAddresses, func(address netip.Addr) bool {
		return address.Is4() || address.Is4In6()
	})
	addresses6 := common.Filter(destinationAddresses, func(address netip.Addr) bool {
		return address.Is6() && !address.Is4In6()
	})
	primaries, fallbacks := addresses4, addresses6
	if preferIPv6 {
		primaries, fallbacks = addresses6, addresses4
	}
	addresses := make([]netip.Addr, 0, len(destinationAddresses))
	for i := 0; i < len(primaries) || i < len(fallbacks); i++ {
		if i < len(primaries) {
			addresses = append(addresses, primaries[i])
		}
		if i < len(fallbacks) {
			addresses = append(addresses, fallbacks[i])
		}
	}
	type dialResult struct {
		net.Conn
		error
	}
	results := make(chan dialResult)
	returned := make(chan struct{})
	defer close(returned)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var (
		started      int
		running      int
		connErrors   []error
		attemptDelay <-chan time.Time
	)
	startAttempt := func() {
		address := addresses[started]
		started++
		running++
		if started < len(addresses) {
			attemptDelay = time.After(fallbackDelay)
		} else {
			attemptDelay = nil
		}
		go func() {
			conn, err := dialer.DialContext(ctx, network, M.SocksaddrFrom(address, destination.Port))
			select {
			case results <- dialResult{Conn: conn, error: err}:
			case <-returned:
				if conn != nil {
					conn.Close()
				}
			}
		}()
	}
	startAttempt()
	for {
		select {
		case <-attemptDelay:
			startAttempt()
		case result := <-results:
			running--
			if result.error == nil {
				return result.Conn, nil
			}
			connErrors = append(connErrors, result.error)
			if started < len(addresses) {
				startAttempt()
			} else if running == 0 {
				return nil, E.Errors(connErrors...)
			}
		}
	}
}
