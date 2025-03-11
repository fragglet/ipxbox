
local rott = Proto("rott", "ROTT netgame protocol")

local rott_rottipx_seq = ProtoField.uint32("rott.rottipx_seq", "Driver sequence number")

local setup_command_types = {
    [1] = "cmd_FindClient",
    [2] = "cmd_HereIAm",
    [3] = "cmd_YouAre",
    [4] = "cmd_IAm",
    [5] = "cmd_AllDone",
    [6] = "cmd_Info",
}

local rott_setup = ProtoField.new("Setup data", "rott.setup", ftypes.BYTES)
local rott_setup_client = ProtoField.int16("rott.setup.client", "Client number")
local rott_setup_playernumber = ProtoField.int16("rott.setup.playernumber", "Player number")
local rott_setup_command = ProtoField.int16("rott.setup.command", "Command number", base.DEC, setup_command_types)
local rott_setup_extra = ProtoField.int16("rott.setup.extra", "Extra data")
local rott_setup_numplayers = ProtoField.int16("rott.setup.numplayers", "Number of players")

rott.fields = { rott_rottipx_seq, rott_setup, rott_setup_client,
                rott_setup_playernumber, rott_setup_command, rott_setup_extra,
                rott_setup_numplayers }

function rott.dissector(tvbuf, pktinfo, root)
    pktinfo.cols.protocol:set("ROTT netgame")

    local pktlen = tvbuf:captured_len()

    if pktlen < 8 then
        -- TODO: Add expert info
        print("packet length", pktlen, "too short")
        return
    end

    local tree = root:add(rott, tvbuf:range(0,pktlen))
    tree:add_le(rott_rottipx_seq, tvbuf:range(0, 4))

    local pkt_bytes = tvbuf:bytes()

    -- Setup packet?
    if pkt_bytes:uint(0, 4) == 0xffffffff then
        local setup_tree = tree:add(rott_setup, tvbuf:range(4, 12))
        setup_tree:add_le(rott_setup_client, tvbuf:range(4, 2))
        setup_tree:add_le(rott_setup_playernumber, tvbuf:range(6, 2))
        setup_tree:add_le(rott_setup_command, tvbuf:range(8, 2))
        setup_tree:add_le(rott_setup_extra, tvbuf:range(10, 2))
        setup_tree:add_le(rott_setup_numplayers, tvbuf:range(12, 2))
    end
end

DissectorTable.get("ipx.socket"):add(0xabcd, rott)
