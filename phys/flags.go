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

type Flags struct {
	Bridge          *Spec
	EthernetFraming *Framer
}

func RegisterFlags() *Flags {
	f := &Flags{}
	f.Bridge = SpecFlag("bridge", "", `Bridge to physical network. Valid values are: "tap:" or "pcap:{device name}"`)
	f.EthernetFraming = FramingTypeFlag("ethernet_framing", `Framing to use when sending Ethernet packets. Valid values are "auto", "802.2", "802.3raw", "snap" and "eth-ii".`)
	return f
}

func (f *Flags) EthernetStream(captureNonIPX bool) (DuplexEthernetStream, error) {
	return f.Bridge.Type(f.Bridge.Arg, captureNonIPX)
}

func (f *Flags) MakePhys(captureNonIPX bool) (*Phys, error) {
	stream, err := f.EthernetStream(captureNonIPX)
	if err != nil {
		return nil, err
	} else if stream != nil {
		return NewPhys(stream, *f.EthernetFraming), nil
	}
	// Physical capture not enabled.
	return nil, nil
}

type Spec struct {
	Type Type
	Arg  string
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
	result = &automaticFramer{fallback: Framer802_2}
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
	return &result
}
