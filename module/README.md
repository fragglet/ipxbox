The ipxbox server is constructed of modules. A module implements some kind of functionality on an IPX network - such a server or some kind of external connectivity. 

Modules are given an IPX network that they can create nodes on for connectivity. This is usually the internal network in an ipxbox server, but it might be a client for a remote server instead. This allows modules to be run standalone, connecting to any dosbox protocol server. 

Implementations of the module interface:

* [aggregate](aggregate/) combines multiple modules into a single module
* [bridge](bridge/) connects the network to a real, physical network (using TAP or libpcap)
* [ipxpkt](ipxpkt/) provides a bridge/router for the ipxpkt.com ethernet-over-IPX driver
* [pptp](pptp/) provides VPN "dial in" over the Point-to-Point Tunneling Protocol
* [qproxy](qproxy/) makes external Quake servers appear on the network (as though they're on the local network segment)
* [server](server/) is the dosbox server itself! 
