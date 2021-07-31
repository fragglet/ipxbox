package lcp

import (
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
)

const PPPTypeIPXCP = layers.PPPType(0x802B)

var LayerTypeIPXCP = gopacket.RegisterLayerType(1819, gopacket.LayerTypeMetadata{
	Name:    "IPXCP",
	Decoder: gopacket.DecodeFunc(decodeIPXCP),
})

// TODO: Implement SerializeTo and make this SerializableLayer.
var _ = gopacket.Layer(&LCP{})

var (
	OptionIPXNetwork               = OptionType{ipxcpDialect, 1}
	OptionIPXNode                  = OptionType{ipxcpDialect, 2}
	OptionIPXCompressionProtocol   = OptionType{ipxcpDialect, 3}
	OptionIPXRoutingProtocol       = OptionType{ipxcpDialect, 4}
	OptionIPXRouterName            = OptionType{ipxcpDialect, 5}
	OptionIPXConfigurationComplete = OptionType{ipxcpDialect, 6}
)

// IPXCP is a gopacket layer for the PPP IPX Control Protocol.
type IPXCP struct {
	BaseLayer
}

func (l *IPXCP) LayerType() gopacket.LayerType {
	return LayerTypeIPXCP
}

func decodeIPXCP(data []byte, p gopacket.PacketBuilder) error {
	ipxcp := &IPXCP{}
	ipxcp.dialect = ipxcpDialect
	if err := ipxcp.UnmarshalBinary(data); err != nil {
		return err
	}
	p.AddLayer(ipxcp)
	return nil
}
