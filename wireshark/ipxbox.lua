-- This file doesn't contain an actual dissector, it just hooks in the
-- ipxbox default UDP port to the Wireshark built-in IPX dissector.
-- You can get the same effect by using "Decode As..." within the UI.
local ipx_dissector = Dissector.get("ipx")
DissectorTable.get("udp.port"):add(10000, ipx_dissector)
