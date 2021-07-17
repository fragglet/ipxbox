package pptp

import (
	"encoding/binary"
	"fmt"
	"net"
)

const (
	pptpPort = 1723
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
	conn net.Conn
}

func (c *Connection) handleStartControl(msg []byte) {
}

func (c *Connection) handleEcho(msg []byte) {
}

func (c *Connection) handleOutgoingCall(msg []byte) {
}

func (c *Connection) readNextMessage() ([]byte, error) {
	var lenField [2]byte
	if _, err := c.conn.Read(lenField[:]); err != nil {
		return nil, err
	}
	msglen := binary.BigEndian.Uint16(lenField[:])
	if msglen < 16 {
		return nil, fmt.Errorf("message too short: len=%d", msglen)
	}
	result := make([]byte, 0, msglen - 2)
	if _, err := c.conn.Read(result); err != nil {
		return nil, err
	}
	gotMagicNumber := binary.BigEndian.Uint32(result[2:6])
	if magicNumber != gotMagicNumber {
		return nil, fmt.Errorf("wrong magic number, want=%x, got=%x", magicNumber, gotMagicNumber)
	}
	return result, nil
}

func (c *Connection) run() {
	for {
		msg, err := c.readNextMessage()
		if err != nil {
			// TODO: log?
			break
		}
		msgtype := binary.BigEndian.Uint16(msg[0:2])
		switch msgtype {
			case msgStartControlConnectionRequest:
				c.handleStartControl(msg)
			case msgEchoRequest:
				c.handleEcho(msg)
			case msgOutgoingCallRequest:
				c.handleOutgoingCall(msg)
		}
	}
}

func newConnection(conn net.Conn) {
}

type Server struct {
	listener *net.TCPListener
}

func (s *Server) Run() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			break
		}
		go newConnection(conn)
	}
	s.listener.Close()
}

func NewServer() (*Server, error) {
	listener, err := net.ListenTCP("tcp", &net.TCPAddr{
		Port: pptpPort,
	})
	if err != nil {
		return nil, err
	}
	return &Server{
		listener: listener,
	}, nil
}

