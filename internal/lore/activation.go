package lore

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
)

const systemdListenFDStart = 3

// ListenServiceSocket adopts one systemd socket or creates a protected socket.
func ListenServiceSocket(socketPath string) (net.Listener, error) {
	listener, activated, err := systemdListener()
	if err != nil {
		return nil, err
	}

	if activated {
		return listener, nil
	}

	return ListenUnix(socketPath)
}

func systemdListener() (net.Listener, bool, error) {
	activated, err := parseSystemdActivation(
		os.Getpid(),
		os.Getenv("LISTEN_PID"),
		os.Getenv("LISTEN_FDS"),
	)
	if err != nil || !activated {
		return nil, false, err
	}

	file := os.NewFile(systemdListenFDStart, "lore.socket")
	if file == nil {
		return nil, false, fmt.Errorf("adopt systemd socket: descriptor is unavailable")
	}

	listener, listenerErr := listenerFromFile(file)
	closeErr := file.Close()

	if err := errors.Join(listenerErr, closeErr); err != nil {
		if listener != nil {
			return nil, false, errors.Join(err, listener.Close())
		}

		return nil, false, err
	}

	return listener, true, nil
}

func parseSystemdActivation(processID int, listenPID, listenFDs string) (bool, error) {
	if listenPID == "" && listenFDs == "" {
		return false, nil
	}

	pid, err := strconv.Atoi(listenPID)
	if err != nil {
		return false, fmt.Errorf("parse LISTEN_PID: %w", err)
	}

	descriptorCount, err := strconv.Atoi(listenFDs)
	if err != nil {
		return false, fmt.Errorf("parse LISTEN_FDS: %w", err)
	}

	if pid != processID {
		return false, nil
	}

	if descriptorCount != 1 {
		return false, fmt.Errorf("adopt systemd socket: expected one descriptor, got %d", descriptorCount)
	}

	return true, nil
}

func listenerFromFile(file *os.File) (net.Listener, error) {
	listener, err := net.FileListener(file)
	if err != nil {
		return nil, fmt.Errorf("adopt systemd socket: %w", err)
	}

	if _, ok := listener.(*net.UnixListener); !ok {
		return nil, errors.Join(fmt.Errorf("adopt systemd socket: descriptor is not a Unix listener"), listener.Close())
	}

	return listener, nil
}
