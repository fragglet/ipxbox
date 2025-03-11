
local doom = Proto("doom", "Doom netgame protocol")

local doom_flags = ProtoField.new("Flags", "doom.flags", ftypes.UINT32, nil, base.HEX)
local doom_flag_exit = ProtoField.bool("doom.flag_exit", "Exit", 32, nil, 0x00000080)
local doom_flag_retransmit = ProtoField.bool("doom.flag_retransmit", "Requesting retransmit", 32, nil, 0x00000040)
local doom_flag_setup = ProtoField.bool("doom.flag_setup", "Setup packet", 32, nil, 0x00000020)
local doom_flag_kill = ProtoField.bool("doom.flag_kill", "Kill game", 32, nil, 0x00000010)
local doom_flag_checksum = ProtoField.uint32("doom.checksum", "Packet checksum", base.HEX, nil, 0xffffff0f)

local doom_retransmitfrom = ProtoField.uint8("doom.retransmitfrom", "Retransmit start tic#")
local doom_starttic = ProtoField.uint8("doom.starttic", "First tic in packet (low byte)")
local doom_player = ProtoField.uint8("doom.player", "Player number")
local doom_numtics = ProtoField.uint8("doom.numtics", "Number of tics in packet")
local doom_tic = ProtoField.new("Tic", "doom.tic", ftypes.BYTES)

local doom_tic_forwardmove = ProtoField.int8("doom.tic.forwardmove", "Forward/backward player movement")
local doom_tic_sidemove = ProtoField.int8("doom.tic.sidemove", "Sideways player movement")
local doom_tic_angleturn = ProtoField.int16("doom.tic.angleturn", "Rotation/turn angle")
local doom_tic_consistency = ProtoField.uint16("doom.tic.consistency", "Consistency check value")
local doom_tic_chatchar = ProtoField.int8("doom.tic.chatchar", "Multiplayer chat character")
local doom_tic_buttons = ProtoField.int8("doom.tic.buttons", "Buttons/special actions")

doom.fields = { doom_flags, doom_flag_exit, doom_flag_retransmit,
                doom_flag_setup, doom_flag_kill, doom_flag_checksum,
                doom_retransmitfrom, doom_starttic, doom_player,
                doom_numtics, doom_tic, doom_tic_forwardmove,
                doom_tic_sidemove, doom_tic_chatchar, doom_tic_angleturn,
                doom_tic_consistency, doom_tic_buttons }

function doom.dissector(tvbuf, pktinfo, root)
    pktinfo.cols.protocol:set("Doom netgame")

    local pktlen = tvbuf:captured_len()

    if pktlen < 8 then
        -- TODO: Add expert info
        print("packet length", pktlen, "too short")
        return
    end

    local tree = root:add(doom, tvbuf:range(0,pktlen))

    local flags_range = tvbuf:range(0, 4)
    local flag_tree = tree:add(doom_flags, flags_range)
    flag_tree:add(doom_flag_checksum, flags_range)
    flag_tree:add(doom_flag_exit, flags_range)
    flag_tree:add(doom_flag_retransmit, flags_range)
    flag_tree:add(doom_flag_setup, flags_range)
    flag_tree:add(doom_flag_kill, flags_range)

    tree:add(doom_retransmitfrom, tvbuf:range(4, 1))
    tree:add(doom_starttic, tvbuf:range(5, 1))
    tree:add(doom_player, tvbuf:range(6, 1))
    tree:add(doom_numtics, tvbuf:range(7, 1))

    local pkt_bytes = tvbuf:bytes()
    local starttic = pkt_bytes:get_index(5)
    local numtics = pkt_bytes:get_index(7)

    local tics_range = tvbuf(8)
    for i = 0, numtics-1 do
        local tic_range = tics_range:range(i * 8, 8)
        local title = string.format("Tic %d", starttic + i)
        local tic = tree:add(doom_tic, tic_range, "", title)
        tic:add(doom_tic_forwardmove, tic_range:range(0, 1))
        tic:add(doom_tic_sidemove, tic_range:range(1, 1))
        tic:add(doom_tic_angleturn, tic_range:range(2, 2))
        tic:add(doom_tic_consistency, tic_range:range(4, 2))
        tic:add(doom_tic_chatchar, tic_range:range(6, 1))
        tic:add(doom_tic_buttons, tic_range:range(7, 1))
    end
end

-- The ipxsetup driver adds a few bytes of data surrounding the "proper" Doom
-- network protocol data (as understood by the game itself):

local doom_ipx = Proto("doom_ipx", "Doom IPX protocol")
local ipxsetup_seq = ProtoField.uint32("doom_ipx.ipxsetup_seq", "ipxsetup sequence ID")
local ipxsetup_tail = ProtoField.uint32("doom_ipx.ipxsetup_tail", "ipxsetup tailing bytes (always zero)")

doom_ipx.fields = { ipxsetup_seq, ipxsetup_tail }

function doom_ipx.dissector(tvbuf, pktinfo, root)
    pktinfo.cols.protocol:set("Doom IPX protocol")

    local pktlen = tvbuf:reported_length_remaining()

    if pktlen < 8 then
        -- TODO: Add expert info
        print("packet length", pktlen, "too short")
        return
    end

    local tree = root:add(doom_ipx, tvbuf:range(0, 4))
    tree:add(ipxsetup_seq, tvbuf:range(0, 4))
    tree:add(ipxsetup_tail, tvbuf:range(pktlen - 4, 4))
    doom.dissector(tvbuf(4, pktlen - 8):tvb(), pktinfo, root)
end

DissectorTable.get("ipx.socket"):add(0x869b, doom_ipx)

