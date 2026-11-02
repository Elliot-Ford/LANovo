//go:build !linux

package chargebase

import "errors"

func openPort(string) (port, error) {
	return nil, errors.New("the charging base is only reachable on Linux")
}
