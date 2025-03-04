package phys

import (
	"flag"
	"fmt"
	"log"
	"strings"
)

// Type is a function that will open a DuplexEthernetStream connection to a
// physical network.
type Type func(arg string, captureNonIPX bool) (DuplexEthernetStream, error)

var (
	types = map[string]Type{
		"tap":  openTap,
		"slirp": openSlirp,
	}
)

type Spec struct {
	Type Type
	Arg  string
}

func (s *Spec) EthernetStream(captureNonIPX bool) (DuplexEthernetStream, error) {
	return s.Type(s.Arg, captureNonIPX)
}

func typeNames() string {
	result := []string{}
	for key := range types {
		result = append(result, fmt.Sprintf("%#v", key))
	}
	return strings.Join(result, ", ")
}

func openNull(arg string, captureNonIPX bool) (DuplexEthernetStream, error) {
	return nil, nil
}

func SpecFlag(name, defaultValue, usage string) *Spec {
	result := &Spec{openNull, ""}
	setValue := func(s string) error {
		parts := strings.SplitN(s, ":", 2)
		if len(parts) == 1 {
			parts = []string{parts[0], ""}
		}
		t, ok := types[parts[0]]
		if !ok {
			return fmt.Errorf("unknown capture type %#v: valid values: %v", parts[0], typeNames())
		}
		result.Type = t
		result.Arg = parts[1]
		return nil
	}
	if defaultValue != "" {
		if err := setValue(defaultValue); err != nil {
			log.Fatalf("BUG: invalid default value %#v for flag %#v", defaultValue, name)
		}
	}
	flag.Func(name, usage, setValue)
	return result
}

func framerNames() string {
	result := []string{}
	for _, f := range allFramers {
		result = append(result, fmt.Sprintf("%q", f.Name()))
	}
	return strings.Join(result, ", ")
}

func FramingTypeFlag(name, usage string) *Framer {
	var result Framer
	setValue := func(s string) error {
		for _, framer := range allFramers {
			if s == framer.Name() {
				result = framer
				return nil
			}
		}
		return fmt.Errorf("unknown Ethernet framing %q: valid values are: %v.", s, framerNames())
	}
	flag.Func(name, usage, setValue)
	if err := setValue("auto"); err != nil {
		log.Fatalf("BUG: failed to set default framing type")
	}
	return &result
}
