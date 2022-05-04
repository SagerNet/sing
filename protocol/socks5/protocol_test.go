package socks5_test

import (
	"net"
	"sync"
	"testing"

	M "github.com/sagernet/sing/common/metadata"
	"github.com/sagernet/sing/protocol/socks5"
)

func TestHandshake(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	wg := new(sync.WaitGroup)
	wg.Add(1)

	method := socks5.AuthTypeUsernamePassword

	go func() {
		response, err := socks5.ClientHandshake(client, socks5.Version5, socks5.CommandConnect, M.AddrPortFrom(M.AddrFromFqdn("test"), 80), "user", "pswd")
		if err != nil {
			t.Fatal(err)
		}
		if response.ReplyCode != socks5.ReplyCodeSuccess {
			t.Fatal(response)
		}
		wg.Done()
	}()
	authRequest, err := socks5.ReadAuthRequest(server)
	if err != nil {
		t.Fatal(err)
	}
	if len(authRequest.Methods) != 1 || authRequest.Methods[0] != method {
		t.Fatal("bad methods: ", authRequest.Methods)
	}
	err = socks5.WriteAuthResponse(server, &socks5.AuthResponse{
		Version: socks5.Version5,
		Method:  method,
	})
	if err != nil {
		t.Fatal(err)
	}
	usernamePasswordAuthRequest, err := socks5.ReadUsernamePasswordAuthRequest(server)
	if err != nil {
		t.Fatal(err)
	}
	if usernamePasswordAuthRequest.Username != "user" || usernamePasswordAuthRequest.Password != "pswd" {
		t.Fatal(authRequest)
	}
	err = socks5.WriteUsernamePasswordAuthResponse(server, &socks5.UsernamePasswordAuthResponse{
		Status: socks5.UsernamePasswordStatusSuccess,
	})
	if err != nil {
		t.Fatal(err)
	}
	request, err := socks5.ReadRequest(server)
	if err != nil {
		t.Fatal(err)
	}
	if request.Version != socks5.Version5 || request.Command != socks5.CommandConnect || request.Destination.Addr.Fqdn() != "test" || request.Destination.Port != 80 {
		t.Fatal(request)
	}
	err = socks5.WriteResponse(server, &socks5.Response{
		Version:   socks5.Version5,
		ReplyCode: socks5.ReplyCodeSuccess,
		Bind:      M.AddrPortFrom(M.AddrFromIP(net.IPv4zero), 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	wg.Wait()
}
