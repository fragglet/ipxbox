package lcp

import (
	"github.com/google/gopacket/layers"
)

const PPPTypeIPXCP = layers.PPPType(0x802B)

var (
	OptionIPXNetwork               = OptionType(1)
	OptionIPXNode                  = OptionType(2)
	OptionIPXCompressionProtocol   = OptionType(3)
	OptionIPXRoutingProtocol       = OptionType(4)
	OptionIPXRouterName            = OptionType(5)
	OptionIPXConfigurationComplete = OptionType(6)
)
