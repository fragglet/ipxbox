`ipxbox` is a standalone DOSBox IPX server written in Go. DOSBox clients can
connect to the server and play together.

A unique feature is that it is optionally able to bridge to real physical
networks, in a manner similar to a VPN. DOSBox clients can communicate with
each other on the server, but with this feature enabled they can also
communicate with physical IPX nodes on the connected network. So emulated DOS
clients should be able to play games against real DOS machines connected to
the same network.

