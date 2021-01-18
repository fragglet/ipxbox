// +build !windows,!plan9,!nacl

package syslog

import (
	"log"
	"log/syslog"
)

// NewLogger creates a log.Logger whose output is written to the
// system log service with the specified priority, a combination of
// the syslog facility and severity. The logFlag argument is the flag
// set passed through to log.New to create the Logger.
// If syslog is not available on this platform then ErrNotImplemented
// is returned.
func NewLogger(p Priority, logFlag int) (*log.Logger, error) {
	return syslog.NewLogger(syslog.Priority(p), logFlag)
}
