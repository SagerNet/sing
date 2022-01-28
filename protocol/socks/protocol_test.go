package socks_test

import (
	"net"
	"sync"
	"testing"

	"sing/common/socksaddr"
	"sing/protocol/socks"
)

func TestHandshake(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	wg := new(sync.WaitGroup)
	wg.Add(1)

	method := socks.AuthTypeUsernamePassword

	go func() {
		response, err := socks.ClientHandshake(client, socks.Version5, socks.CommandConnect, socksaddr.AddrFromFqdn("test"), 80, "user", "pswd")
		if err != nil {
			t.Fatal(err)
		}
		if response.ReplyCode != socks.ReplyCodeSuccess {
			t.Fatal(response)
		}
		wg.Done()
	}()
	authRequest, err := socks.ReadAuthRequest(server)
	if err != nil {
		t.Fatal(err)
	}
	if len(authRequest.Methods) != 1 || authRequest.Methods[0] != method {
		t.Fatal("bad methods: ", authRequest.Methods)
	}
	err = socks.WriteAuthResponse(server, &socks.AuthResponse{
		Version: socks.Version5,
		Method:  method,
	})
	if err != nil {
		t.Fatal(err)
	}
	usernamePasswordAuthRequest, err := socks.ReadUsernamePasswordAuthRequest(server)
	if err != nil {
		t.Fatal(err)
	}
	if usernamePasswordAuthRequest.Username != "user" || usernamePasswordAuthRequest.Password != "pswd" {
		t.Fatal(authRequest)
	}
	err = socks.WriteUsernamePasswordAuthResponse(server, &socks.UsernamePasswordAuthResponse{
		Status: socks.UsernamePasswordStatusSuccess,
	})
	if err != nil {
		t.Fatal(err)
	}
	request, err := socks.ReadRequest(server)
	if err != nil {
		t.Fatal(err)
	}
	if request.Version != socks.Version5 || request.Command != socks.CommandConnect || request.Addr.Fqdn() != "test" || request.Port != 80 {
		t.Fatal(request)
	}
	err = socks.WriteResponse(server, &socks.Response{
		Version:   socks.Version5,
		ReplyCode: socks.ReplyCodeSuccess,
		BindAddr:  socksaddr.AddrFromIP(net.IPv4zero),
		BindPort:  0,
	})
	if err != nil {
		t.Fatal(err)
	}
	wg.Wait()
}
