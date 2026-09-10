package lore

import (
	"context"
	"net"
	"path/filepath"
	"strconv"
	"testing"
)

func TestParseSystemdActivation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		processID int
		listenPID string
		listenFDs string
		want      bool
		wantErr   bool
	}{
		{name: "absent"},
		{name: "one descriptor", processID: 42, listenPID: "42", listenFDs: "1", want: true},
		{name: "other process", processID: 42, listenPID: "41", listenFDs: "1"},
		{name: "several descriptors", processID: 42, listenPID: "42", listenFDs: "2", wantErr: true},
		{name: "invalid process", processID: 42, listenPID: "invalid", listenFDs: "1", wantErr: true},
		{name: "invalid descriptors", processID: 42, listenPID: "42", listenFDs: "invalid", wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseSystemdActivation(test.processID, test.listenPID, test.listenFDs)
			if test.wantErr && err == nil {
				t.Fatal("expected activation error")
			}

			if !test.wantErr && (err != nil || got != test.want) {
				t.Fatalf("parseSystemdActivation() = %v, %v", got, err)
			}
		})
	}
}

func TestListenerFromFileServesRequests(t *testing.T) {
	t.Parallel()

	socketPath := filepath.Join(t.TempDir(), "lore.sock")

	original, err := net.ListenUnix("unix", &net.UnixAddr{Name: socketPath, Net: "unix"})
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	original.SetUnlinkOnClose(false)

	file, err := original.File()
	if err != nil {
		t.Fatalf("duplicate listener descriptor: %v", err)
	}

	if err := original.Close(); err != nil {
		t.Fatalf("close original listener: %v", err)
	}

	listener, err := listenerFromFile(file)
	if err != nil {
		t.Fatalf("listenerFromFile() error = %v", err)
	}

	if err := file.Close(); err != nil {
		t.Fatalf("close descriptor: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)

	handler := RequestHandlerFunc(func(context.Context, Request) Response {
		return SuccessResponse(struct {
			Activated bool `json:"activated"`
		}{Activated: true})
	})

	go func() {
		done <- NewServer(handler).Serve(ctx, listener)
	}()

	var result struct {
		Activated bool `json:"activated"`
	}
	if err := NewClient(socketPath).Call(context.Background(), "test", struct{}{}, &result); err != nil {
		t.Fatalf("Call() error = %v", err)
	}

	if !result.Activated {
		t.Fatal("activated listener result is false")
	}

	cancel()

	if err := <-done; err != nil {
		t.Fatalf("Serve() error = %v", err)
	}
}

func TestParseSystemdActivationCurrentProcess(t *testing.T) {
	t.Parallel()

	processID := 1234

	activated, err := parseSystemdActivation(processID, strconv.Itoa(processID), "1")
	if err != nil || !activated {
		t.Fatalf("parseSystemdActivation() = %v, %v", activated, err)
	}
}
