`ipxbox` is a standalone DOSBox IPX server written in Go. DOSBox clients can
connect to the server and play together.

Also included is ipxbridge, which implements an IPX bridge server - DOSBox
clients can connect to the server and packets will be bridged to a real
physical network. So emulated DOS clients should be able to play against
real DOS machines connected to the same network.

## Setting up an IPX network bridge

The following explains how to set up an IPX bridge server on Linux. This could
be as simple as a Raspberry Pi that you connect to a network along with a
retro DOS machine (Raspberry Pis are cheap and ubiquitous nowadays).

First compile `ipxbox` and run with the `--enable_tap` flag (probably requires
root privileges, hence sudo):

 go build ipxbox.go
 sudo ./ipxbox --port=10000 --enable_tap

If successful the server should start with no problems and there will be a
`tap0` device listed in the output of `ifconfig`.

 $ ifconfig
 [...]
 tap0: flags=4163<UP,BROADCAST,RUNNING,MULTICAST>  mtu 1500
         inet 169.254.194.27  netmask 255.255.0.0  broadcast 169.254.255.255
         inet6 fe80::e055:4e79:b7a3:6b7a  prefixlen 64  scopeid 0x20<link>
         ether 16:b6:35:40:d9:65  txqueuelen 1000  (Ethernet)
         RX packets 172  bytes 9440 (9.2 KiB)
         RX errors 0  dropped 27  overruns 0  frame 0
         TX packets 145  bytes 38878 (37.9 KiB)
         TX errors 0  dropped 0 overruns 0  carrier 0  collisions 0

The `tap0` device is an isolated virtual network connected to any physical
network. To connect to a physical network, you need to create a bridge device
that bridges `tap0` with a physical network. The following will create a
bridge named `br0` that bridges `tap0` with a physical network on adapter
`eth0`:

 # ip link add name br0 type bridge
 # ip link set br0 up
 # ip link set eth0 master br0
 # ip link set tap0 master br0

To test, use `tcpdump` to view IPX packets on the bridge device, eg.

 # tcpdump -b ipx -nli br0

Connect to the server with DOSBox, eg. if the server is at address 10000,

 C:\>config -set ipx true
 C:\>ipxnet connect 192.168.1.104 10000
 IPX Tunneling utility for DosBox

 IPX Tunneling Client connected to server at 192.168.1.104.

You should see periodic IPX ping packets on the bridge device, and the
addresses will begin with 02:..., for example:

 22:12:00.290972 (NOV-ETHII) IPX 00000000.02:56:84:7a:fe:97.0002 > 00000000.02:ff:ff:ff:00:00.0002: ipx-#2 0
 22:12:05.307859 (NOV-ETHII) IPX 00000000.02:56:84:7a:fe:97.0002 > 00000000.02:ff:ff:ff:00:00.0002: ipx-#2 0

