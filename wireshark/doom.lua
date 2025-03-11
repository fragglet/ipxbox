
local NCMD_EXIT = 0x80000000
local NCMD_RETRANSMIT = 0x40000000
local NCMD_SETUP = 0x20000000
local NCMD_KILL = 0x10000000
local NCMD_CHECKSUM = 0x0fffffff

local doom = Proto("doom", "Doom netgame protocol")

local doom_flags = ProtoField.new("Flags", "doom.flags", ftypes.UINT32, nil, base.HEX)
local doom_flag_exit = ProtoField.bool("doom.flag_exit", "Exit", 32, nil, NCMD_EXIT)
local doom_flag_retransmit = ProtoField.bool("doom.flag_retransmit", "Requesting retransmit", 32, nil, NCMD_RETRANSMIT)
local doom_flag_setup = ProtoField.bool("doom.flag_setup", "Setup packet", 32, nil, NCMD_SETUP)
local doom_flag_kill = ProtoField.bool("doom.flag_kill", "Kill game", 32, nil, NCMD_KILL)
local doom_flag_checksum = ProtoField.uint32("doom.checksum", "Packet checksum", base.HEX, nil, NCMD_CHECKSUM)

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
    local flag_tree = tree:add_le(doom_flags, flags_range)
    flag_tree:add_le(doom_flag_checksum, flags_range)
    flag_tree:add_le(doom_flag_exit, flags_range)
    flag_tree:add_le(doom_flag_retransmit, flags_range)
    flag_tree:add_le(doom_flag_setup, flags_range)
    flag_tree:add_le(doom_flag_kill, flags_range)

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

local ipxsetup_gameid = ProtoField.uint16("doom_ipx.gameid", "Game ID (unused)")
local ipxsetup_drone = ProtoField.uint16("doom_ipx.drone", "Drone (unused)")
local ipxsetup_nodesfound = ProtoField.uint16("doom_ipx.nodesfound", "Number of nodes found")
local ipxsetup_nodeswanted = ProtoField.uint16("doom_ipx.nodeswanted", "Number of nodes wanted")
local ipxsetup_dupwanted = ProtoField.uint16("doom_ipx.dupwanted", "Ticdup wanted (xttl extension)")
local ipxsetup_plnumwanted = ProtoField.uint16("doom_ipx.plnumwanted", "Player number wanted (xttl extension)")

doom_ipx.fields = { ipxsetup_seq, ipxsetup_tail, ipxsetup_gameid,
                    ipxsetup_drone, ipxsetup_nodesfound, ipxsetup_nodeswanted,
                    ipxsetup_dupwanted, ipxsetup_plnumwanted }

function doom_ipx.dissector(tvbuf, pktinfo, root)
    pktinfo.cols.protocol:set("Doom IPX protocol")

    local pktlen = tvbuf:captured_len()

    if pktlen < 8 then
        -- TODO: Add expert info
        print("packet length", pktlen, "too short")
        return
    end

    local pkt_bytes = tvbuf:bytes()
    local tree = root:add(doom_ipx, tvbuf:range(0, 4))
    tree:add_le(ipxsetup_seq, tvbuf:range(0, 4))
    tree:add_le(ipxsetup_tail, tvbuf:range(pktlen - 4, 4))

    if pkt_bytes:uint(0, 4) == 0xffffffff then
        tree:add_le(ipxsetup_gameid, tvbuf:range(4, 2))
        tree:add_le(ipxsetup_drone, tvbuf:range(6, 2))
        tree:add_le(ipxsetup_nodesfound, tvbuf:range(8, 2))
        tree:add_le(ipxsetup_nodeswanted, tvbuf:range(10, 2))
        if pktlen >= 20 then
            tree:add_le(ipxsetup_dupwanted, tvbuf:range(12, 2))
            tree:add_le(ipxsetup_plnumwanted, tvbuf:range(14, 2))
        end
    else
        doom.dissector(tvbuf(4, pktlen - 8):tvb(), pktinfo, root)
    end
end

DissectorTable.get("ipx.socket"):add(0x869b, doom_ipx)

