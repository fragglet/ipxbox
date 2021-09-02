![ipxbox icon](images/ipxbox4x.png)

`ipxbox` is a standalone DOSBox IPX server written in Go.

Features:

* TUN/TAP and libpcap integration that allows the server to be bridged to a
real, physical network. This allows (emulated) dosbox users to play alongside
users on real DOS machines (see [BRIDGE-HOWTO](BRIDGE-HOWTO.md) for more
information).

* Built-in PPTP server that allows Windows 9x users to connect over the
Internet  using the VPN software that shipped with the operating system
([demo video](https://www.youtube.com/watch?v=ut37z6EE5Hc);
see [PPTP-HOWTO](PPTP-HOWTO.md) for more information).

* Support for the `ipxpkt.com` packet driver protocol, allowing
TCP/IP-over IPX; software that uses the packet driver interface can
more easily be used in DOSbox
([demo video](https://www.youtube.com/watch?v=5VeVaFbORhI)).

* Sends background keepalive pings to idle DOSbox clients to prevent users
behind NAT routers from being timed out.

* Proxying to Quake servers, so that you can make UDP-based Quake servers
appear as "local" IPX servers
([demo video](https://www.youtube.com/watch?v=SB3JOjdhJHI)).

* Syslog integration for audit logging when running a public server.

For some setup instructions, see the [HOWTO](HOWTO.md).

