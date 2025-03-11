
local numTrailBytes = 32

local ipxpkt = Proto("ipxpkt", "ipxpkt.com tunneling protocol")

local ipxpkt_fragment = ProtoField.uint8("ipxpkt.fragment", "Packet fragment number (1-indexed)")
local ipxpkt_num_fragments = ProtoField.uint8("ipxpkt.num_fragments", "Number of fragments")
local ipxpkt_packet_number = ProtoField.uint16("ipxpkt.packet_number", "Packet number")
local ipxpkt_payload = ProtoField.new("Payload", "ipxpkt.payload", ftypes.BYTES)

local ethernet_dissector = Dissector.get("eth_withoutfcs")

ipxpkt.fields = { ipxpkt_fragment, ipxpkt_num_fragments, ipxpkt_packet_number,
                  ipxpkt_payload }

function ipxpkt.dissector(tvbuf, pktinfo, root)
    local pktlen = tvbuf:captured_len()

    if pktlen < numTrailBytes + 4 then
        -- TODO: Add expert info
        print("packet length", pktlen, "too short")
        return
    end

    local header_buf = tvbuf:range(numTrailBytes, 4)
    local header_bytes = header_buf:bytes()
    local tree = root:add(ipxpkt, header_buf)

    tree:add(ipxpkt_fragment, header_buf:range(0, 1))
    tree:add(ipxpkt_num_fragments, header_buf:range(1, 1))
    tree:add(ipxpkt_packet_number, header_buf:range(2, 2))
    tree:add(ipxpkt_payload, tvbuf(numTrailBytes + 4))

    -- TODO: There should probably be a config option to only decode complete
    -- frames (no fragments)
    if header_bytes:get_index(1) == 1 then
        ethernet_dissector:call(tvbuf(numTrailBytes + 4):tvb(), pktinfo, root)
    end
end

DissectorTable.get("ipx.socket"):add(0x6181, ipxpkt)
