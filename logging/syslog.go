//go:build !windows && !plan9 && !nacl

package logging

import (
	"log/slog"
	"log/syslog"

	slogsyslog "github.com/samber/slog-syslog/v2"
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

	handler := slogsyslog.Option{
		Level:  slog.LevelInfo,
		Writer: writer,
	}.NewSyslogHandler()
	return handler, nil
}
