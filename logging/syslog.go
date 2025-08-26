//go:build !windows && !plan9 && !nacl

package logging

import (
	"log/slog"
	"log/syslog"
)

const (
	priority = syslog.LOG_WARNING | syslog.LOG_DAEMON
	tag      = "ipxbox"
)

type syslogType struct {
}

func (syslogType) makeHandler(arg string) (slog.Handler, error) {
	var (
		writer *syslog.Writer
		err    error
	)
	if arg != "" {
		writer, err = syslog.Dial("tcp", arg, priority, tag)
	} else {
		writer, err = syslog.New(priority, tag)
	}
	if err != nil {
		return nil, err
	}

	// We use slog's built-in plain text handler to format the log
	// messages sent to syslog. The only thing we don't send is the
	// timestamp, since that's recorded by the syslogd anyway.
	return slog.NewTextHandler(writer, &slog.HandlerOptions{
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == "time" {
				return slog.Attr{}
			} else {
				return a
			}
		},
	}), nil
}
