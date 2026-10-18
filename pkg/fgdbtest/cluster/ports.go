// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

package cluster

import (
	"fmt"
	"net"
	"strings"
)

const (
	portAttempts = 5
	bindText     = "address already in use"
	listenFormat = "127.0.0.1:%d"
)

func assignPorts(n int) (sql []int, http []int, err error) {
	ports, listeners, err := reserve(n * 2)
	closeListeners(listeners)
	if err != nil {
		return nil, nil, err
	}
	return ports[:n], ports[n:], nil
}

func reserve(n int) ([]int, []net.Listener, error) {
	ports := make([]int, 0, n)
	listeners := make([]net.Listener, 0, n)
	for len(ports) < n {
		ln, err := net.Listen("tcp", fmt.Sprintf(listenFormat, 0))
		if err != nil {
			closeListeners(listeners)
			return nil, nil, err
		}
		listeners = append(listeners, ln)
		ports = append(ports, ln.Addr().(*net.TCPAddr).Port)
	}
	return ports, listeners, nil
}

func closeListeners(listeners []net.Listener) {
	for _, ln := range listeners {
		if ln != nil {
			_ = ln.Close()
		}
	}
}

func bindConflict(err error) bool {
	return err != nil && strings.Contains(err.Error(), bindText) && !strings.Contains(err.Error(), survivorText)
}
