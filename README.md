ipxbox is a standalone DOSBox IPX server written in Go. DOSBox clients can
connect to the server and play together.

Also included is ipxbridge, which implements an IPX bridge server - DOSBox
clients can connect to the server and packets will be bridged to a real
physical network. So emulated DOS clients should be able to play against
real DOS machines connected to the same network.

TODO: Add instructions for how to run a bridge server.

