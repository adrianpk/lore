package lore

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	// ProtocolVersion identifies the initial local IPC protocol.
	ProtocolVersion = "lore/v1"
	// MaxRequestSize is the largest accepted encoded request.
	MaxRequestSize = 1 << 20
	// MaxResponseSize is the largest accepted encoded response.
	MaxResponseSize = 8 << 20

	defaultConnectionTimeout = 10 * time.Second
)

// ErrorCode identifies one stable service failure category.
type ErrorCode string

const (
	// ErrorInvalidRequest indicates malformed or structurally invalid input.
	ErrorInvalidRequest ErrorCode = "invalid_request"
	// ErrorUnsupportedProtocol indicates an unknown protocol version.
	ErrorUnsupportedProtocol ErrorCode = "unsupported_protocol"
	// ErrorUnknownOperation indicates an unsupported operation name.
	ErrorUnknownOperation ErrorCode = "unknown_operation"
	// ErrorNotFound indicates that no requested entry exists.
	ErrorNotFound ErrorCode = "not_found"
	// ErrorInvalidEntry indicates an entry contract violation.
	ErrorInvalidEntry ErrorCode = "invalid_entry"
	// ErrorJournalCorrupt indicates invalid canonical journal state.
	ErrorJournalCorrupt ErrorCode = "journal_corrupt"
	// ErrorJournalUnavailable indicates inaccessible journal storage.
	ErrorJournalUnavailable ErrorCode = "journal_unavailable"
	// ErrorInternal indicates an unexpected service failure.
	ErrorInternal ErrorCode = "internal_error"
)

// Request is one bounded local service operation.
type Request struct {
	Protocol  string          `json:"protocol"`
	Operation string          `json:"operation"`
	Params    json.RawMessage `json:"params"`
}

// Response is one bounded local service result or error.
type Response struct {
	Protocol string          `json:"protocol"`
	OK       bool            `json:"ok"`
	Result   json.RawMessage `json:"result,omitempty"`
	Error    *ServiceError   `json:"error,omitempty"`
}

// ServiceError is a stable machine-readable service failure.
type ServiceError struct {
	Code    ErrorCode `json:"code"`
	Message string    `json:"message"`
}

// Error returns the service failure message.
func (serviceError *ServiceError) Error() string {
	if serviceError == nil {
		return ""
	}

	return string(serviceError.Code) + ": " + serviceError.Message
}

// RequestHandler executes one validated request.
type RequestHandler interface {
	Handle(context.Context, Request) Response
}

// RequestHandlerFunc adapts a function to RequestHandler.
type RequestHandlerFunc func(context.Context, Request) Response

// Handle executes the adapted function.
func (handler RequestHandlerFunc) Handle(ctx context.Context, request Request) Response {
	return handler(ctx, request)
}

// SuccessResponse encodes one successful operation result.
func SuccessResponse(result any) Response {
	encoded, err := json.Marshal(result)
	if err != nil {
		return ErrorResponse(ErrorInternal, "encode service result")
	}

	return Response{Protocol: ProtocolVersion, OK: true, Result: encoded}
}

// ErrorResponse constructs one stable service error response.
func ErrorResponse(code ErrorCode, message string) Response {
	return Response{
		Protocol: ProtocolVersion,
		OK:       false,
		Error:    &ServiceError{Code: code, Message: message},
	}
}

// Server handles one request per local stream connection.
type Server struct {
	handler           RequestHandler
	connectionTimeout time.Duration
}

// NewServer stores the handler without performing IO.
func NewServer(handler RequestHandler) *Server {
	return &Server{handler: handler, connectionTimeout: defaultConnectionTimeout}
}

// ListenUnix creates one protected local Unix stream listener.
func ListenUnix(socketPath string) (*net.UnixListener, error) {
	if socketPath == "" {
		return nil, fmt.Errorf("listen on Unix socket: path is required")
	}

	runtimeDirectory := filepath.Dir(socketPath)
	if err := os.MkdirAll(runtimeDirectory, 0o700); err != nil {
		return nil, fmt.Errorf("create runtime directory %q: %w", runtimeDirectory, err)
	}

	if err := os.Chmod(runtimeDirectory, 0o700); err != nil {
		return nil, fmt.Errorf("set runtime directory permissions %q: %w", runtimeDirectory, err)
	}

	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: socketPath, Net: "unix"})
	if err != nil {
		return nil, fmt.Errorf("listen on Unix socket %q: %w", socketPath, err)
	}

	if err := os.Chmod(socketPath, 0o600); err != nil {
		return nil, errors.Join(fmt.Errorf("set Unix socket permissions %q: %w", socketPath, err), listener.Close())
	}

	return listener, nil
}

// Serve accepts connections until the context is canceled or the listener fails.
func (server *Server) Serve(ctx context.Context, listener net.Listener) error {
	if server.handler == nil {
		return fmt.Errorf("serve local requests: handler is required")
	}

	var connections sync.WaitGroup

	stopClosing := make(chan struct{})

	closeErrors := make(chan error, 1)
	go closeListenerOnCancel(ctx, listener, stopClosing, closeErrors)

	defer close(stopClosing)
	defer connections.Wait()

	for {
		connection, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return <-closeErrors
			}

			return fmt.Errorf("accept local connection: %w", err)
		}

		connections.Add(1)
		go func() {
			defer connections.Done()

			server.serveConnection(ctx, connection)
		}()
	}
}

func closeListenerOnCancel(ctx context.Context, listener net.Listener, stop <-chan struct{}, closeErrors chan<- error) {
	select {
	case <-ctx.Done():
		err := listener.Close()
		if errors.Is(err, net.ErrClosed) {
			err = nil
		}

		closeErrors <- err
	case <-stop:
	}
}

func (server *Server) serveConnection(ctx context.Context, connection net.Conn) {
	defer closeConnection(connection)

	if err := setConnectionDeadline(connection, ctx, server.connectionTimeout); err != nil {
		return
	}

	request, response := readRequest(connection)
	if response.Error != nil {
		if err := writeResponse(connection, response); err != nil {
			return
		}

		return
	}

	response = server.handler.Handle(ctx, request)
	if response.Protocol == "" {
		response.Protocol = ProtocolVersion
	}

	if err := writeResponse(connection, response); err != nil {
		return
	}
}

func closeConnection(connection net.Conn) {
	if err := connection.Close(); err != nil {
		return
	}
}

func readRequest(reader io.Reader) (Request, Response) {
	encoded, tooLarge, err := readBounded(reader, MaxRequestSize)
	if err != nil {
		return Request{}, ErrorResponse(ErrorInvalidRequest, "read request")
	}

	if tooLarge {
		return Request{}, ErrorResponse(ErrorInvalidRequest, "request exceeds the allowed size")
	}

	request, err := decodeRequest(encoded)
	if err != nil {
		return Request{}, ErrorResponse(ErrorInvalidRequest, err.Error())
	}

	if request.Protocol != ProtocolVersion {
		return Request{}, ErrorResponse(ErrorUnsupportedProtocol, "unsupported protocol")
	}

	return request, Response{}
}

func decodeRequest(encoded []byte) (Request, error) {
	var request Request

	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&request); err != nil {
		return Request{}, fmt.Errorf("decode request: %w", err)
	}

	if err := requireJSONEnd(decoder); err != nil {
		return Request{}, err
	}

	if request.Protocol == "" {
		return Request{}, fmt.Errorf("protocol is required")
	}

	if request.Operation == "" {
		return Request{}, fmt.Errorf("operation is required")
	}

	if len(request.Params) == 0 || !json.Valid(request.Params) {
		return Request{}, fmt.Errorf("params must contain valid JSON")
	}

	return request, nil
}

func writeResponse(writer io.Writer, response Response) error {
	encoded, err := json.Marshal(response)
	if err != nil {
		return fmt.Errorf("encode response: %w", err)
	}

	encoded = append(encoded, '\n')
	if len(encoded) > MaxResponseSize {
		encoded, err = json.Marshal(ErrorResponse(ErrorInternal, "response exceeds the allowed size"))
		if err != nil {
			return fmt.Errorf("encode response size error: %w", err)
		}

		encoded = append(encoded, '\n')
	}

	if err := writeAll(writer, encoded); err != nil {
		return fmt.Errorf("write response: %w", err)
	}

	return nil
}

func readBounded(reader io.Reader, maximum int) ([]byte, bool, error) {
	encoded, err := io.ReadAll(io.LimitReader(reader, int64(maximum)+1))
	if err != nil {
		return nil, false, err
	}

	return encoded, len(encoded) > maximum, nil
}

func writeAll(writer io.Writer, data []byte) error {
	for len(data) > 0 {
		written, err := writer.Write(data)
		if err != nil {
			return err
		}

		if written == 0 {
			return io.ErrShortWrite
		}

		data = data[written:]
	}

	return nil
}

func setConnectionDeadline(connection net.Conn, ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	if contextDeadline, ok := ctx.Deadline(); ok && contextDeadline.Before(deadline) {
		deadline = contextDeadline
	}

	if err := connection.SetDeadline(deadline); err != nil {
		return fmt.Errorf("set local connection deadline: %w", err)
	}

	return nil
}

// Client sends bounded requests to the local Lore service.
type Client struct {
	socketPath string
	timeout    time.Duration
}

// NewClient stores the local socket path without performing IO.
func NewClient(socketPath string) *Client {
	return &Client{socketPath: socketPath, timeout: defaultConnectionTimeout}
}

// Call performs one local operation and decodes its successful result.
func (client *Client) Call(ctx context.Context, operation string, params, result any) error {
	request, err := encodeClientRequest(operation, params)
	if err != nil {
		return err
	}

	dialer := net.Dialer{Timeout: client.timeout}

	connection, err := dialer.DialContext(ctx, "unix", client.socketPath)
	if err != nil {
		return fmt.Errorf("connect to Lore service: %w", err)
	}

	if err := setConnectionDeadline(connection, ctx, client.timeout); err != nil {
		return closeClientConnection(connection, err)
	}

	if err := writeAll(connection, request); err != nil {
		return closeClientConnection(connection, fmt.Errorf("write Lore request: %w", err))
	}

	unixConnection, ok := connection.(*net.UnixConn)
	if !ok {
		return closeClientConnection(connection, fmt.Errorf("close Lore request stream: connection is not Unix"))
	}

	if err := unixConnection.CloseWrite(); err != nil {
		return closeClientConnection(connection, fmt.Errorf("close Lore request stream: %w", err))
	}

	responseErr := readClientResponse(connection, result)

	return closeClientConnection(connection, responseErr)
}

func encodeClientRequest(operation string, params any) ([]byte, error) {
	encodedParams, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("encode Lore request params: %w", err)
	}

	request := Request{Protocol: ProtocolVersion, Operation: operation, Params: encodedParams}

	encoded, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("encode Lore request: %w", err)
	}

	encoded = append(encoded, '\n')
	if len(encoded) > MaxRequestSize {
		return nil, fmt.Errorf("encode Lore request: request exceeds the allowed size")
	}

	return encoded, nil
}

func readClientResponse(reader io.Reader, result any) error {
	encoded, tooLarge, err := readBounded(reader, MaxResponseSize)
	if err != nil {
		return fmt.Errorf("read Lore response: %w", err)
	}

	if tooLarge {
		return fmt.Errorf("read Lore response: response exceeds the allowed size")
	}

	var response Response

	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&response); err != nil {
		return fmt.Errorf("decode Lore response: %w", err)
	}

	if err := requireJSONEnd(decoder); err != nil {
		return err
	}

	if err := validateResponse(response); err != nil {
		return err
	}

	if !response.OK {
		return response.Error
	}

	if result == nil {
		return nil
	}

	if err := json.Unmarshal(response.Result, result); err != nil {
		return fmt.Errorf("decode Lore result: %w", err)
	}

	return nil
}

func validateResponse(response Response) error {
	if response.Protocol != ProtocolVersion {
		return fmt.Errorf("decode Lore response: unsupported protocol %q", response.Protocol)
	}

	if response.OK {
		if response.Error != nil || len(response.Result) == 0 {
			return fmt.Errorf("decode Lore response: invalid success envelope")
		}

		return nil
	}

	if response.Error == nil || response.Error.Code == "" || response.Error.Message == "" || len(response.Result) != 0 {
		return fmt.Errorf("decode Lore response: invalid error envelope")
	}

	return nil
}

func closeClientConnection(connection net.Conn, operationErr error) error {
	return errors.Join(operationErr, connection.Close())
}
