// +build windows plan9 nacl

package syslog

import (
	"log"
)

func NewLogger(p Priority, logFlag int) (*log.Logger, error) {
	return nil, ErrNotImplemented
}
