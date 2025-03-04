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
	}
)

type Flags struct {
	Bridge          *Spec
	EthernetFraming *string
}

func RegisterFlags() *Flags {
	f := &Flags{}
	f.Bridge = SpecFlag("bridge", "", `Bridge to physical network. Valid values are: "tap:" or "pcap:{device name}"`)
	f.EthernetFraming = flag.String("ethernet_framing", "auto", `Framing to use when sending Ethernet packets. Valid values are "auto", "802.2", "802.3raw", "snap" and "eth-ii".`)
	return f
}

func (f *Flags) EthernetStream(captureNonIPX bool) (DuplexEthernetStream, error) {
	return f.Bridge.Type(f.Bridge.Arg, captureNonIPX)
}

func (f *Flags) makeFramer() (Framer, error) {
	framerName := *f.EthernetFraming
	if framerName == "auto" {
		return &automaticFramer{
			fallback: Framer802_2,
		}, nil
	}
	for _, framer := range allFramers {
		if framerName == framer.Name() {
			return framer, nil
		}
	}
	return nil, fmt.Errorf("unknown Ethernet framing %q", framerName)
}

func (f *Flags) MakePhys(captureNonIPX bool) (*Phys, error) {
	stream, err := f.EthernetStream(captureNonIPX)
	if err != nil {
		return nil, err
	} else if stream != nil {
		framer, err := f.makeFramer()
		if err != nil {
			return nil, err
		}
		return NewPhys(stream, framer), nil
	}
	// Physical capture not enabled.
	return nil, nil
}

type Spec struct {
	Type Type
	Arg  string
}

func typeNames() []string {
	result := []string{}
	for key := range types {
		result = append(result, key)
	}
	return result
}

func SpecFlag(name, defaultValue, usage string) *Spec {
	result := &Spec{}
	setValue := func(s string) error {
		parts := strings.SplitN(s, ":", 2)
		if len(parts) != 2 {
			return fmt.Errorf("failed to parse flag value: %#v", s)
		}
		t, ok := types[parts[0]]
		if !ok {
			return fmt.Errorf("unknown capture type %#v: valid values: %#v", parts[0], typeNames())
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
