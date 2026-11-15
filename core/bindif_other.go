//go:build !darwin && !linux && !android

package core

import (
	"errors"
	"syscall"
)

func bindControl(_ string) func(network, address string, c syscall.RawConn) error {
	return nil
}

func listenBoundICMP(_ string) (icmpPacketConn, error) {
	return nil, errors.New("interface-bound ICMP not supported on this OS")
}
