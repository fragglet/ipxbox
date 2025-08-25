//go:build windows || plan9 || nacl

package logging

import (
	"errors"
	"log/slog"
)

var noSyslogError = errors.New("syslog logging is not supported on this platform")

type syslogType struct {
}

func (syslogType) makeHandler(arg string) (slog.Handler, error) {
	return nil, noSyslogError
}
