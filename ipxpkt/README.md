This is an implementation of the protocol used by IPXPKT.COM, a packet
driver included with the Crynwr packet driver collection. This packet
driver emulates an Ethernet network by tunneling frames inside IPX
packets.

This packet driver is useful for a couple of reasons. Firstly, vanilla
DOSbox (at the time of writing, at least) only supports IPX networking.
The ipxpkt driver therefore will allow DOS programs that use the packet
driver interface to be used inside DOSbox. Secondly, it should also
allow packet driver-based DOS programs to be used inside Win9x, as a
virtual Ethernet link can be established over IPX to another machine
on the network.

