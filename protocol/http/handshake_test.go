package http

import (
	std_http "net/http"
	"testing"
)

func TestCanKeepAliveHTTPProxyResponseRejectsCloseDelimitedHTTP10Body(t *testing.T) {
	request := &std_http.Request{
		Method:     std_http.MethodGet,
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header:     std_http.Header{"Proxy-Connection": []string{"keep-alive"}},
	}

	response := &std_http.Response{
		StatusCode:    std_http.StatusUnauthorized,
		ProtoMajor:    1,
		ProtoMinor:    0,
		Header:        std_http.Header{"WWW-Authenticate": []string{`Basic realm="K2P"`}},
		ContentLength: -1,
		Close:         true,
	}

	if !requestWantsHTTPProxyKeepAlive(request) {
		t.Fatal("request should ask for HTTP proxy keep-alive")
	}
	if canKeepAliveHTTPProxyResponse(request, response) {
		t.Fatal("HTTP proxy must not keep alive a close-delimited HTTP/1.0 response")
	}
}

func TestCanKeepAliveHTTPProxyResponseAllowsLengthDelimitedBody(t *testing.T) {
	request := &std_http.Request{
		Method:     std_http.MethodGet,
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header:     std_http.Header{"Proxy-Connection": []string{"keep-alive"}},
	}

	response := &std_http.Response{
		StatusCode:    std_http.StatusOK,
		ProtoMajor:    1,
		ProtoMinor:    1,
		ContentLength: 2,
		Close:         false,
	}

	if !canKeepAliveHTTPProxyResponse(request, response) {
		t.Fatal("HTTP proxy should keep alive a length-delimited reusable response")
	}
}

func TestResponseHasCloseDelimitedBodyIgnoresNoBodyResponses(t *testing.T) {
	request := &std_http.Request{Method: std_http.MethodGet}

	for _, statusCode := range []int{
		std_http.StatusContinue,
		std_http.StatusSwitchingProtocols,
		std_http.StatusNoContent,
		std_http.StatusNotModified,
	} {
		response := &std_http.Response{
			StatusCode:    statusCode,
			ContentLength: -1,
		}
		if responseHasCloseDelimitedBody(request, response) {
			t.Fatalf("status %d must not be treated as close-delimited body", statusCode)
		}
	}
}
