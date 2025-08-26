package logging

import (
	"flag"
	"fmt"
	"log"
	"log/slog"
	"os"
	"strings"
)

type Type interface {
	makeHandler(arg string) (slog.Handler, error)
}

var (
	TypeNone   = &noneType{}
	TypeStdout = &stdoutType{}
	TypeSyslog = &syslogType{}

	loggingTypes = map[string]Type{
		"none":   TypeNone,
		"stdout": TypeStdout,
		"syslog": TypeSyslog,
	}
)

type noneType struct {
}

func (noneType) makeHandler(arg string) (slog.Handler, error) {
	return slog.DiscardHandler, nil
}

type stdoutType struct {
}

func (stdoutType) makeHandler(arg string) (slog.Handler, error) {
	return slog.NewTextHandler(os.Stdout, nil), nil
}

type Spec struct {
	Type Type
	Arg  string
}

func (s *Spec) MakeLogger() (*slog.Logger, error) {
	handler, err := s.Type.makeHandler(s.Arg)
	if err != nil {
		return nil, err
	}
	return slog.New(handler), nil
}

// TODO: Delete this function
func (s *Spec) MakeLogLogger() (*log.Logger, error) {
	handler, err := s.Type.makeHandler(s.Arg)
	if err != nil {
		return nil, err
	}
	return slog.NewLogLogger(handler, slog.LevelInfo), nil
}

func MakeFlag() *Spec {
	spec := &Spec{TypeNone, ""}
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
