// Package pptp contains an implementation of a PPTP VPN server that is
// specifically intended to allow IPX protocol games to be played from old
// Windows 9x machines. It is deliberately limited in scope and functionality,
// lacking many of the features commonly found in most PPTP implementations
// that are not necessary for its intended function.
package pptp

import (
	"encoding/binary"
	"fmt"
	"net"

	"github.com/fragglet/ipxbox/network"
	"github.com/fragglet/ipxbox/ppp"
)

const (
	pptpPort    = 1723
	magicNumber = 0x1a2b3c4d
)

const (
	msgStartControlConnectionRequest = iota + 1
	msgStartControlConnectionReply
	msgStopControlConnectionRequest
	msgStopControlConnectionReply
	msgEchoRequest
	msgEchoReply
	msgOutgoingCallRequest
	msgOutgoingCallReply
	msgIncomingCallRequest
	msgIncomingCallReply
	msgIncomingCallConnected
	msgCallClearRequest
	msgCallDisconnectNotify
	msgWanErrorNotify
	msgSetLinkInfo
)

type Connection struct {
	callID uint16
	conn   net.Conn
	ppp    *ppp.Session
	s      *Server
}

func (c *Connection) sendMessage(msg []byte) {
	msg = append([]byte{0, 0}, msg...)
	binary.BigEndian.PutUint16(msg[0:2], uint16(len(msg)))
	c.conn.Write(msg)
}

func (c *Connection) handleStartControl(msg []byte) {
	// We literally don't care about anything they sent us.
	reply := []byte{
		0x00, 0x01, // Message type
		0x1a, 0x2b, 0x3c, 0x4d, // Magic cookie
		0x00, 0x02, // Control message type
		0x00, 0x00, // Reserved0
		0x01, 0x00, // Protocol version
		0x01,                   // Result code
		0x00,                   // Error code
		0x00, 0x00, 0x00, 0x00, // Framing capability
		0x00, 0x00, 0x00, 0x00, // Bearer capability
		0x00, 0x01, // Maximum channels
		0x00, 0x01, // Firmware revision
	}
	var (
		hostname [64]byte
		vendor   [64]byte
	)
	copy(hostname[:], []byte("server"))
	copy(vendor[:], []byte("ipxbox"))
	reply = append(reply, hostname[:]...)
	reply = append(reply, vendor[:]...)
	c.sendMessage(reply)
}

func (c *Connection) handleEcho(msg []byte) {
	reply := []byte{
		0x00, 0x01, // Message type
		0x1a, 0x2b, 0x3c, 0x4d, // Magic cookie
		0x00, 0x06, // Control message type
		0x00, 0x00, // Reserved0
		0xff, 0xff, 0xff, 0xff, // Identifier
		0x01,       // Result code
		0x00,       // Error code
		0x00, 0x00, // Reserved1
	}
	// Send back the same identifier:
	copy(reply[10:14], msg[10:14])
	c.sendMessage(reply)
}

func (c *Connection) Close() error {
	err1 := c.conn.Close()
	var err2 error
	if c.ppp != nil {
		err2 = c.ppp.Close()
	}
	switch {
	case err1 != nil:
		return err1
	case err2 != nil:
		return err2
	default:
		return nil
	}
}

func (c *Connection) startPPPSession(sendCallID uint16) {
	if c.ppp != nil {
		return
	}
	addr := c.conn.RemoteAddr().(*net.TCPAddr)
	var err error
	gre, err := startGRESession(addr.IP, sendCallID, c.callID)
	if err != nil {
		// TODO: Send back error message? Log error?
		c.conn.Close()
		return
	}
	node := c.s.n.NewNode()
	c.ppp = ppp.NewSession(gre, node)
	go func() {
		err := c.ppp.Run()
		if err != nil {
			// TODO: log error?
		}
		// Once the PPP session terminates, close the PPTP control
		// connection as well.
		c.Close()
	}()
}

func (c *Connection) handleOutgoingCall(msg []byte) {
	if len(msg) < 22 {
		return
	}
	// Start up GRE session if we have not already.
	sendCallID := binary.BigEndian.Uint16(msg[10:12])
	c.startPPPSession(sendCallID)
	reply := []byte{
		0x00, 0x01, // Message type
		0x1a, 0x2b, 0x3c, 0x4d, // Magic cookie
		0x00, 0x08, // Control message type
		0x00, 0x00, // Reserved0
		0x01, 0x80, // Call ID
		0x00, 0x00, // Peer call ID
		0x01,       // Result code
		0x00,       // Error code
		0x00, 0x00, // Cause code
		0x00, 0x00, 0xfa, 0x00, // Connect speed
		0x00, 0x10, // Receive window size
		0x00, 0x00, // Processing delay
		0x00, 0x00, 0x00, 0x00, // Physical channel ID
	}
	// Call ID.
	binary.BigEndian.PutUint16(reply[10:12], c.callID)
	// We deliberately set the receive window size to a large value (1024
	// packets). We never want the client to stop sending packets because
	// it's waiting for an ack. For the old games we're targeting, we can
	// expect to never reach the bandwidth limit on modern networks.
	binary.BigEndian.PutUint16(reply[22:24], 1024)
	// Connect speed = maximum speed. Same deal.
	copy(reply[18:22], msg[18:22])
	// Copy peer's call ID.
	copy(reply[12:14], msg[10:12])
	c.sendMessage(reply)
}

func (c *Connection) readNextMessage() ([]byte, error) {
	var lenField [2]byte
	if _, err := c.conn.Read(lenField[:]); err != nil {
		return nil, err
	}
	msglen := binary.BigEndian.Uint16(lenField[:])
	switch {
	case msglen < 16:
		return nil, fmt.Errorf("message too short: len=%d", msglen)
	case msglen > 256:
		return nil, fmt.Errorf("message too long: len=%d", msglen)
	}
	result := make([]byte, msglen-2)
	if _, err := c.conn.Read(result); err != nil {
		return nil, err
	}
	gotMsgType := binary.BigEndian.Uint16(result[0:2])
	if gotMsgType != 1 {
		return nil, fmt.Errorf("wrong PPTP message type, want=1, got=%d", gotMsgType)
	}
	gotMagicNumber := binary.BigEndian.Uint32(result[2:6])
	if magicNumber != gotMagicNumber {
		return nil, fmt.Errorf("wrong magic number, want=%x, got=%x", magicNumber, gotMagicNumber)
	}
	return result, nil
}

func (c *Connection) run() {
messageLoop:
	for {
		// TODO: Send periodic Echo-Requests to keep the TCP stream
		// alive?
		msg, err := c.readNextMessage()
		if err != nil {
			// TODO: log?
			break
		}
		msgtype := binary.BigEndian.Uint16(msg[6:8])
		switch msgtype {
		case msgStartControlConnectionRequest:
			c.handleStartControl(msg)
		case msgEchoRequest:
			c.handleEcho(msg)
		case msgOutgoingCallRequest:
			c.handleOutgoingCall(msg)
		case msgCallClearRequest:
			break messageLoop
		}
	}
	if c.ppp != nil {
		c.ppp.Terminate(fmt.Errorf("PPTP control connection closed by client"))
	}
	c.Close()
}

func newConnection(s *Server, conn net.Conn, callID uint16) *Connection {
	return &Connection{
		s:      s,
		conn:   conn,
		callID: callID,
	}
}

// Server is an implementation of a PPTP server.
type Server struct {
	listener   *net.TCPListener
	nextCallID uint16
	n          network.Network
}

// Run listens for and accepts new connections to the server. It blocks until
// the server is shut down, so it should be invoked in a dedicated goroutine.
func (s *Server) Run() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			break
		}
		c := newConnection(s, conn, s.nextCallID)
		go c.run()
		s.nextCallID = (s.nextCallID + 1) & 0xffff
	}
	s.listener.Close()
}

func (s *Server) Close() error {
	return s.listener.Close()
}

func NewServer(n network.Network) (*Server, error) {
	listener, err := net.ListenTCP("tcp", &net.TCPAddr{
		Port: pptpPort,
	})
	if err != nil {
		return nil, err
	}
	return &Server{
		listener:   listener,
		nextCallID: 384,
		n:          n,
	}, nil
}
