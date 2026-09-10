package lore

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClientServerRoundTrip(t *testing.T) {
	t.Parallel()

	handler := RequestHandlerFunc(func(_ context.Context, request Request) Response {
		if request.Operation != "echo" {
			return ErrorResponse(ErrorUnknownOperation, "unknown operation")
		}

		return Response{Protocol: ProtocolVersion, OK: true, Result: request.Params}
	})

	client, socketPath := startTransportServer(t, handler)

	params := struct {
		Message string `json:"message"`
	}{Message: "bounded local request"}

	var result struct {
		Message string `json:"message"`
	}

	if err := client.Call(context.Background(), "echo", params, &result); err != nil {
		t.Fatalf("Call() error = %v", err)
	}

	if result != params {
		t.Fatalf("Call() result = %#v, want %#v", result, params)
	}

	assertSocketPermissions(t, socketPath)
}

func TestClientReturnsServiceError(t *testing.T) {
	t.Parallel()

	handler := RequestHandlerFunc(func(context.Context, Request) Response {
		return ErrorResponse(ErrorUnknownOperation, "unknown operation")
	})
	client, _ := startTransportServer(t, handler)

	err := client.Call(context.Background(), "missing", struct{}{}, nil)

	var serviceError *ServiceError
	if !errors.As(err, &serviceError) {
		t.Fatalf("Call() error = %v, want ServiceError", err)
	}

	if serviceError.Code != ErrorUnknownOperation {
		t.Fatalf("error code = %q, want %q", serviceError.Code, ErrorUnknownOperation)
	}
}

func TestReadRequestValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		request  string
		wantCode ErrorCode
	}{
		{
			name:    "valid",
			request: `{"protocol":"lore/v1","operation":"record","params":{}}`,
		},
		{
			name:     "unsupported protocol",
			request:  `{"protocol":"lore/v2","operation":"record","params":{}}`,
			wantCode: ErrorUnsupportedProtocol,
		},
		{
			name:     "unknown envelope field",
			request:  `{"protocol":"lore/v1","operation":"record","params":{},"extra":true}`,
			wantCode: ErrorInvalidRequest,
		},
		{
			name:     "missing operation",
			request:  `{"protocol":"lore/v1","params":{}}`,
			wantCode: ErrorInvalidRequest,
		},
		{
			name:     "trailing JSON",
			request:  `{"protocol":"lore/v1","operation":"record","params":{}} {}`,
			wantCode: ErrorInvalidRequest,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			request, response := readRequest(strings.NewReader(test.request))
			if test.wantCode == "" {
				if response.Error != nil || request.Operation != "record" {
					t.Fatalf("readRequest() = %#v, %#v", request, response)
				}

				return
			}

			if response.Error == nil || response.Error.Code != test.wantCode {
				t.Fatalf("readRequest() response = %#v, want code %q", response, test.wantCode)
			}
		})
	}
}

func TestReadRequestRejectsOversize(t *testing.T) {
	t.Parallel()

	_, response := readRequest(bytes.NewReader(make([]byte, MaxRequestSize+1)))
	if response.Error == nil || response.Error.Code != ErrorInvalidRequest {
		t.Fatalf("readRequest() response = %#v", response)
	}

	if response.Error.Message != "request exceeds the allowed size" {
		t.Fatalf("error message = %q", response.Error.Message)
	}
}

func TestServerBoundsResponse(t *testing.T) {
	t.Parallel()

	handler := RequestHandlerFunc(func(context.Context, Request) Response {
		return SuccessResponse(struct {
			Value string `json:"value"`
		}{Value: strings.Repeat("x", MaxResponseSize)})
	})
	client, _ := startTransportServer(t, handler)

	err := client.Call(context.Background(), "large", struct{}{}, nil)

	var serviceError *ServiceError
	if !errors.As(err, &serviceError) {
		t.Fatalf("Call() error = %v, want ServiceError", err)
	}

	if serviceError.Code != ErrorInternal || serviceError.Message != "response exceeds the allowed size" {
		t.Fatalf("service error = %#v", serviceError)
	}
}

func TestEncodeClientRequestRejectsOversize(t *testing.T) {
	t.Parallel()

	params := struct {
		Value string `json:"value"`
	}{Value: strings.Repeat("x", MaxRequestSize)}

	if _, err := encodeClientRequest("record", params); err == nil {
		t.Fatal("expected request size error")
	}
}

func TestValidateResponse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		response Response
		wantErr  bool
	}{
		{
			name:     "success",
			response: SuccessResponse(struct{}{}),
		},
		{
			name:     "error",
			response: ErrorResponse(ErrorNotFound, "not found"),
		},
		{
			name:     "wrong protocol",
			response: Response{Protocol: "lore/v2", OK: true, Result: json.RawMessage(`{}`)},
			wantErr:  true,
		},
		{
			name:     "success without result",
			response: Response{Protocol: ProtocolVersion, OK: true},
			wantErr:  true,
		},
		{
			name:     "error without detail",
			response: Response{Protocol: ProtocolVersion, OK: false},
			wantErr:  true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			err := validateResponse(test.response)
			if test.wantErr && err == nil {
				t.Fatal("expected validation error")
			}

			if !test.wantErr && err != nil {
				t.Fatalf("validateResponse() error = %v", err)
			}
		})
	}
}

func startTransportServer(t *testing.T, handler RequestHandler) (*Client, string) {
	t.Helper()

	socketPath := filepath.Join(t.TempDir(), "runtime", "lore.sock")

	listener, err := ListenUnix(socketPath)
	if err != nil {
		t.Fatalf("ListenUnix() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)

	server := NewServer(handler)
	go func() {
		done <- server.Serve(ctx, listener)
	}()

	t.Cleanup(func() {
		cancel()

		if err := <-done; err != nil {
			t.Errorf("Serve() error = %v", err)
		}
	})

	return NewClient(socketPath), socketPath
}

func assertSocketPermissions(t *testing.T, socketPath string) {
	t.Helper()

	info, err := os.Stat(socketPath)
	if err != nil {
		t.Fatalf("stat socket: %v", err)
	}

	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("socket mode = %o, want 600", got)
	}

	runtimeInfo, err := os.Stat(filepath.Dir(socketPath))
	if err != nil {
		t.Fatalf("stat runtime directory: %v", err)
	}

	if got := runtimeInfo.Mode().Perm(); got != 0o700 {
		t.Fatalf("runtime directory mode = %o, want 700", got)
	}
}

func TestListenUnixRejectsExistingSocket(t *testing.T) {
	t.Parallel()

	socketPath := filepath.Join(t.TempDir(), "lore.sock")

	listener, err := ListenUnix(socketPath)
	if err != nil {
		t.Fatalf("first ListenUnix() error = %v", err)
	}

	if _, err := ListenUnix(socketPath); err == nil {
		t.Fatal("expected existing socket error")
	}

	if err := listener.Close(); err != nil {
		t.Fatalf("close listener: %v", err)
	}
}

func TestClientRejectsInvalidServerEnvelope(t *testing.T) {
	t.Parallel()

	socketPath := filepath.Join(t.TempDir(), "lore.sock")

	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: socketPath, Net: "unix"})
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	done := make(chan error, 1)

	go func() {
		connection, acceptErr := listener.Accept()
		if acceptErr != nil {
			done <- acceptErr

			return
		}

		_, _, readErr := readBounded(connection, MaxRequestSize)
		if readErr == nil {
			readErr = writeAll(connection, []byte(`{"protocol":"lore/v1","ok":true}`))
		}

		done <- closeClientConnection(connection, readErr)
	}()

	client := NewClient(socketPath)
	if err := client.Call(context.Background(), "test", struct{}{}, nil); err == nil {
		t.Fatal("expected invalid response error")
	}

	if err := <-done; err != nil {
		t.Fatalf("fake server error = %v", err)
	}

	if err := listener.Close(); err != nil {
		t.Fatalf("close listener: %v", err)
	}
}
