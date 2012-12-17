//
// A standalone DOSbox IPX-over-UDP server.
//

package main

import (
	"bytes"
	"fmt"
	"math/rand"
	"net"
	"os"
	"time"
)

type IPXAddr [6]byte

type Client struct {
	addr           *net.UDPAddr
	ipxAddr        IPXAddr
	lastPacketTime time.Time
}

type IPXHeaderAddr struct {
	network [4]byte
	addr    IPXAddr
	socket  uint16
}

type IPXHeader struct {
	checksum     uint16
	length       uint16
	transControl byte
	packetType   byte
	dest, src    IPXHeaderAddr
}

// Clients time out after 10 minutes of inactivity.
const CLIENT_TIMEOUT = 10 * 60 * 1e9

var ADDR_NULL = []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
var ADDR_BROADCAST = []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff}

var serverSocket *net.UDPConn
var clients = map[string]*Client{}
var clientsByIPX = map[IPXAddr]*Client{}

func decodeIPXHeaderAddr(data []byte, addr *IPXHeaderAddr) {
	copy(addr.network[0:], data[0:4])
	copy(addr.addr[0:], data[4:10])
	addr.socket = uint16((data[10] << 8) | data[11])
}

func decodeIPXHeader(packet []byte) *IPXHeader {
	if len(packet) < 30 {
		return nil
	}

	var result IPXHeader

	result.checksum = uint16((packet[0] << 8) | packet[1])
	result.length = uint16((packet[2] << 8) | packet[3])
	result.transControl = packet[4]
	result.packetType = packet[5]

	decodeIPXHeaderAddr(packet[6:18], &result.dest)
	decodeIPXHeaderAddr(packet[18:30], &result.src)

	return &result
}

func encodeIPXHeaderAddr(data []byte, addr *IPXHeaderAddr) {
	copy(data[0:4], addr.network[0:4])
	copy(data[4:10], addr.addr[0:])
	data[10] = byte(addr.socket >> 8)
	data[11] = byte(addr.socket & 0xff)
}

func encodeIPXHeader(header *IPXHeader) []byte {
	result := make([]byte, 30)
	result[0] = byte(header.checksum >> 8)
	result[1] = byte(header.checksum & 0xff)
	result[2] = byte(header.length >> 8)
	result[3] = byte(header.length & 0xff)
	result[4] = header.transControl
	result[5] = header.packetType

	encodeIPXHeaderAddr(result[6:18], &header.dest)
	encodeIPXHeaderAddr(result[18:30], &header.src)

	return result
}

// Having received a packet, forward it on to another client.
func forwardPacket(header *IPXHeader, packet []byte) {
	if client, ok := clientsByIPX[header.dest.addr]; ok {
		serverSocket.WriteToUDP(packet, client.addr)
	}
}

// Having received a broadcast packet, forward it to all clients.
func forwardBroadcastPacket(header *IPXHeader, packet []byte) {
	for _, client := range clients {
		if client.ipxAddr != header.src.addr {
			serverSocket.WriteToUDP(packet, client.addr)
		}
	}
}

// Allocate a new random address that does not share an address with
// an existing client.
func newAddress() IPXAddr {
	var result IPXAddr

	for {
		for i := 0; i < len(result); i++ {
			result[i] = byte(rand.Intn(255))
		}

		if _, ok := clientsByIPX[result]; !ok {
			break
		}
	}

	return result
}

// Process a registration packet, adding a new client if necessary.
func newClient(header *IPXHeader, addr *net.UDPAddr) {
	addrStr := addr.String()
	client, ok := clients[addrStr]

	if !ok {
		now := time.Now()
		//fmt.Printf("%s: %s: New client\n", now, addr)

		client = new(Client)
		client.addr = addr
		client.ipxAddr = newAddress()
		client.lastPacketTime = now

		clients[addrStr] = client
		clientsByIPX[client.ipxAddr] = client
	}

	// Send a reply back to the client
	packet := new(IPXHeader)
	packet.checksum = 0xffff
	packet.length = 30
	packet.transControl = 0

	copy(packet.dest.network[0:], []byte{0, 0, 0, 0})
	copy(packet.dest.addr[0:], client.ipxAddr[0:])
	packet.dest.socket = 2

	copy(packet.src.network[0:], []byte{0, 0, 0, 1})
	copy(packet.src.addr[0:], ADDR_BROADCAST[0:])
	packet.src.socket = 2

	serverSocket.WriteToUDP(encodeIPXHeader(packet), client.addr)
}

func isRegistrationPacket(header *IPXHeader) bool {
	return header.dest.socket == 2 &&
		bytes.Compare(header.dest.addr[0:], ADDR_NULL) == 0
}

func isBroadcast(header *IPXHeader) bool {
	return bytes.Compare(header.dest.addr[0:], ADDR_BROADCAST) == 0
}

// Process a received UDP packet.
func processPacket(packet []byte, addr *net.UDPAddr) {
	header := decodeIPXHeader(packet)
	if header == nil {
		return
	}

	if isRegistrationPacket(header) {
		newClient(header, addr)
		return
	}

	srcClient, ok := clients[addr.String()]
	if !ok {
		return
	}

	// Clients can only send from their own address.
	if bytes.Compare(header.src.addr[0:], srcClient.ipxAddr[0:]) != 0 {
		return
	}

	srcClient.lastPacketTime = time.Now()

	if isBroadcast(header) {
		forwardBroadcastPacket(header, packet)
	} else {
		forwardPacket(header, packet)
	}
}

func createSocket(addr string) *net.UDPConn {
	udp4Addr, err := net.ResolveUDPAddr("up4", addr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to resolve address: ",
			err.Error())
		os.Exit(1)
	}

	socket, err := net.ListenUDP("udp", udp4Addr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open socket: ",
			err.Error())
		os.Exit(1)
	}

	return socket
}

func checkClientTimeouts() {
	now := time.Now()
	for _, client := range clients {
		if now.After(client.lastPacketTime.Add(CLIENT_TIMEOUT)) {
			//fmt.Printf("%s: %s: Client timed out\n",
			//	now, client.addr)
			delete(clients, client.addr.String())
			delete(clientsByIPX, client.ipxAddr)
		}
	}
}

func main() {
	serverSocket = createSocket(":10000")

	timeoutCheckTime := time.Now().Add(10e9)

	for {
		var buf [1500]byte

		serverSocket.SetReadDeadline(timeoutCheckTime)

		packetLen, addr, err := serverSocket.ReadFromUDP(buf[0:])

		if err == nil {
			processPacket(buf[0:packetLen], addr)
		} else if nerr, ok := err.(net.Error); ok && !nerr.Timeout() {
			fmt.Fprintf(os.Stderr, "%s\n", err.Error())
			os.Exit(1)
		}

		if time.Now().After(timeoutCheckTime) {
			checkClientTimeouts()
			timeoutCheckTime = timeoutCheckTime.Add(10e9)
		}
	}
}
