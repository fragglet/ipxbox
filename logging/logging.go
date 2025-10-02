// Package logging provides a command line flag for configuring the logging
// output format for Go's structured logging.
package logging

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
)

// Type represents a type of logging output, and is used to instantiate a
// `slog.Handler` for writing logging output.
type Type interface {
	makeHandler(arg string) (slog.Handler, error)
}

var (
	// TypeNone just discards all log output.
	TypeNone = &noneType{}

	// TypeStdout writes log output to stdout.
	TypeStdout = &stdoutType{}

	// TypeSyslog writes log output to the Unix syslog, or sends it to
	// a remote syslog server.
	TypeSyslog = &syslogType{}

	loggingTypes = map[string]Type{
		"none":   TypeNone,
		"stdout": TypeStdout,
		"syslog": TypeSyslog,
	}
)

// TODO: Switch to slog.DiscardHandler once we migrate to a newer Go version
type discardHandler struct{}

func (h *discardHandler) Enabled(context.Context, slog.Level) bool { return false }
func (h *discardHandler) Handle(context.Context, slog.Record) error { return nil }
func (h *discardHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *discardHandler) WithGroup(string) slog.Handler { return h }

type noneType struct {
}

func (noneType) makeHandler(arg string) (slog.Handler, error) {
	return &discardHandler{}, nil
}

type stdoutType struct {
}

func (stdoutType) makeHandler(arg string) (slog.Handler, error) {
	return slog.NewTextHandler(os.Stdout, nil), nil
}

// Spec is the parsed form of the --logging command line flag, consisting of
// the logging type, plus an optional string argument for instantiating the
// `slog.Handler`.
type Spec struct {
	Type Type
	Arg  string
}

// MakeLogger instantiates an `slog.Logger` based on the logging Spec.
func (s *Spec) MakeLogger() (*slog.Logger, error) {
	handler, err := s.Type.makeHandler(s.Arg)
	if err != nil {
		return nil, err
	}
	return slog.New(handler), nil
}

// RegisterFlag registers the --logging flag, and returns a `Spec` that will be
// populated at flag parsing time.
func RegisterFlag() *Spec {
	spec := &Spec{TypeStdout, ""}
	flag.Func("logging", "log output; options are none; stdout; syslog[:addr]", func(s string) error {
		parts := strings.SplitN(s, ":", 2)
		if len(parts) == 1 {
			parts = append(parts, "")
		}
		t, ok := loggingTypes[parts[0]]
		if !ok {
			return fmt.Errorf("invalid logging output type %q", parts[0])
		}
		spec.Type = t
		spec.Arg = parts[1]
		return nil
	})
	return spec
}
