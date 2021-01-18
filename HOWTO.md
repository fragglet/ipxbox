
## Compiling

If you haven't compiled a Go program before, the following are some brief
instructions to help you compile ipxbox on a Linux-based system.

First, install the Go compiler and the development package for `libpcap`.
On a Debian-based system this will work:
```
sudo apt install golang libpcap-dev
```
Next set `$GOPATH`. This can usually be pointed at a directory inside your
home directory:
```
export GOPATH=$HOME/go
```
You may find it convenient to add the above line to `~/.bashrc`.

The following two commands will fetch ipxbox and all its dependencies, and
then compile it:
```
go get github.com/fragglet/ipxbox
go build github.com/fragglet/ipxbox
```
If successful, you should now have a compiled binary you can run named `ipxbox`.

## Starting a server

To test your server, you can try running it with:
```
./ipxbox --port=10000
```
The server will run on UDP port 10000 until you hit ctrl-c. Try connecting to
it from inside DOSbox. For example, if your server is running locally you can
specify `localhost` as the address to connect to:
```
Z:\>config -set ipx true
Z:\>ipxnet connect localhost 10000
IPX Tunneling utility for DosBox

IPX Tunneling Client connected to server at localhost.

Z:\>
```
If you see that you were able to connect successfully as above, you now know
that the server is working correctly.

If you are trying to connect to a remote machine and it is failing, the
following are two possible causes:

1. If the server is running on a home network, you may need to set up a port
forward. Google search port forwarding to find out more about this subject -
the way to do it depends on your router.

1. Check if a firewall is set up on the machine you are trying to connect to.
If there is, you may need to add a firewall exception. Linux firewalls use
`iptables`, and the following is an example of how to add an exception:
```
sudo iptables -A INPUT --dport 10000 -p udp -j ACCEPT 
```

## Setting up a systemd service

Once you have your server working you may want to set up a `systemd` service
for it. This ensures that if the machine/VM restarts, the server will
automatically restart. In the following example we are setting up a service
running as the user `jonny`.

First create a `systemd` configuration file. You'll need to point the
`ExecStart` path at your executable.
```
mkdir -p ~/.config/systemd/user
cat >~/.config/systemd/user/ipxbox.service <<END
[Unit]
Description=DOSbox dedicated server

[Service]
ExecStart=/home/jonny/ipxbox/ipxbox
Restart=on-failure

[Install]
WantedBy=default.target
END
```
Enable login lingering for `jonny`. This allows the server to run even when
`jonny` isn't logged in.
```
sudo loginctl enable-linger jonny
```
Start the server and enable it as a service that will start automatically.
```
systemctl --user start ipxbox.service
systemctl --user enable ipxbox.service
```
Check it is running as expected:
```
$ systemctl --user status ipxbox.service
● ipxbox.service - DOSbox dedicated server
   Loaded: loaded (/home/jonny/.config/systemd/user/ipxbox.service; enabled; vendor preset: enabled
   Active: active (running) since Sun 2020-02-16 00:00:51 GMT; 3 weeks 6 days ago
 Main PID: 5383 (ipxbox)
   CGroup: /user.slice/user-1001.slice/user@1001.service/ipxbox.service
           └─5383 /home/jonny/ipxbox/ipxbox
```

## Advanced topic: setting up an IPX network bridge

`ipxbox` by default just acts as a forwarding server between DOSbox clients,
but it can be configured to bridge to a real network.

**First, a word of warning**: the DOSBox IPX protocol is completely insecure.
There's no encryption or authentication supported. For this reason, by default
ipxbox blocks the IPX ports associated with Windows filesharing. There's not
a lot of damage you can do with the IPX protocol nowadays but there's still the
possibility that if you use this on a public server, you might be exposing
something on your network that you don't intend to.

There are two ways to set things up: `ipxbox` can either create a TAP device
(use `--enable_tap`) or use `libpcap` to connect to a real Ethernet device.
If you don't know what this means, you'll want to use the `libpcap` approach.

Find out which Ethernet interface (network card) you want to use by using the
Linux `ifconfig` command. Usually the interface will be named something like
`eth0` but it can vary sometimes.

Programs don't usually have the permission to do raw network access. You'll
need to grant it to the `ipxbox` binary:
```
sudo setcap cap_net_raw,cap_net_admin=eip ./ipxbox
```
Then run `ipxbox` with the `--pcap_device` argument, eg.
```
./ipxbox --port=10000 --pcap_device=eth0
```
If working correctly, clients connecting to the server will now be bridged to
`eth0`. You can test this using `tcpdump` to listen for IPX packets and
checking if you see any when a client is connected.
```
$ sudo tcpdump -nli eth0 ipx
tcpdump: verbose output suppressed, use -v or -vv for full protocol decode
listening on eth0, link-type EN10MB (Ethernet), capture size 262144 bytes
05:08:38.891724 IPX 00000000.02:cf:0d:86:54:e5.0002 > 00000000.02:ff:ff:ff:00:00.0002: ipx-#2 0
05:08:43.886672 IPX 00000000.02:cf:0d:86:54:e5.0002 > 00000000.02:ff:ff:ff:00:00.0002: ipx-#2 0
05:08:47.890883 IPX 00000000.02:cf:0d:86:54:e5.4002 > 00000000.ff:ff:ff:ff:ff:ff.6590: ipx-#6590 16
05:08:48.529183 IPX 00000000.02:cf:0d:86:54:e5.4002 > 00000000.ff:ff:ff:ff:ff:ff.6590: ipx-#6590 16
05:08:48.888311 IPX 00000000.02:cf:0d:86:54:e5.0002 > 00000000.02:ff:ff:ff:00:00.0002: ipx-#2 0
```
## Advanced topic: IPX packet driver routing

Much DOS software that communicates over the network (particularly using the
TCP/IP protocol stack used on the Internet) uses the
[packet driver](https://en.wikipedia.org/wiki/PC/TCP_Packet_Driver) interface.
There are packet drivers available for most network cards, providing a
standard interface for sending and receiving data over the network.

Some forks of DOSbox can emulate the Novell NE2000 card, allowing such
software to be used. However, vanilla DOSbox at the time of writing does not
include this feature. Furthermore, it typically requires granting DOSbox
special permission to be able to send and receive raw network packets.
The [`ipxpkt.com`](ipxpkt/driver/) driver is a packet driver that tunnels an
Ethernet link over IPX packets, and ipxbox includes support for its protocol.
This allows DOSbox users to use packet driver-based software.

**First, a word of warning**: the DOSBox IPX protocol is completely insecure.
There's no encryption or authentication supported, and enabling this feature
gives a potential backdoor into the network where the ipxbox server is running.
If you don't understand the implications of this, don't enable this feature
on a public-facing ipxbox server.

To use this feature:

1. First set up an IPX bridge by following the instructions from the previous
section.

2. Read the warning in the paragraph above this list. Then add
`--enable_ipxpkt` to the ipxbox command line. For example:
```
./ipxbox --port=10000 --pcap_device=eth0 --enable_ipxpkt
```
3. Start a DOSbox client and connect to the server as normal. Make sure to
mount a directory containing the [`ipxpkt.com`](ipxpkt/driver/) driver.

4. Start the packet driver as shown:
```
C:\>ipxpkt -i 0x60

Packet driver for IPX, version 11.3
Portions Copyright 1990, P. Kranenburg
Packet driver skeleton copyright 1988-93, Crynwr Software.
This program is freely copyable; source must be available; NO WARRANTY.
See the file COPYING.DOC for details; send FAX to +1-315-268-9201 for a copy.

This may take up to 30 seconds...
System: [345]86 processor, ISA bus, Two 8259s
Packet driver software interrupt is 0x60 (96)
My Ethernet address is 02:57:04:31:68:FA

C:\>
```
5. Test the connection is working correctly by trying some software that uses
the packet driver interface. The [mTCP stack](http://www.brutman.com/mTCP/)
makes for a good first step. For example:
```
C:\>set mtcpcfg=c:\mtcp\mtcp.cfg
C:\>dhcp

mTCP DHCP Client by M Brutman (mbbrutman@gmail.com) (C)opyright 2008-2020
Version: Mar  7 2020

Timeout per request: 10 seconds, Retry attempts: 3
Sending DHCP requests, Press [ESC] to abort.

DHCP request sent, attempt 1: Offer received, Acknowledged

Good news everyone!

IPADDR 192.168.128.45
NETMASK 255.255.0.0
GATEWAY 192.168.60.1
NAMESERVER 8.8.8.8
LEASE_TIME 86400 seconds

Settings written to 'c:\mtcp\mtcp.cfg'

C:\>ping 8.8.8.8

mTCP Ping by M Brutman (mbbrutman@gmail.com) (C)opyright 2009-2020
Version: Mar  7 2020

ICMP Packet payload is 32 bytes.

Packet sequence number 0 received in 45.90 ms, ttl=114
Packet sequence number 1 received in 44.20 ms, ttl=114
Packet sequence number 2 received in 41.65 ms, ttl=114
Packet sequence number 3 received in 54.40 ms, ttl=114

Packets sent: 4, Replies received: 4, Replies lost: 0
Average time for a reply: 46.53 ms (not counting lost packets)
```
