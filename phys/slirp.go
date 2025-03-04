package phys

// Implementation of phys.DuplexEthernetStream that talks to a libslirp
// subprocess, silently NATting any TCP/IP connections through the libslirp
// user-space networking stack.

import (
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/google/gopacket"
)

var (
	_ = (DuplexEthernetStream)(&SlirpConnection{})
)

type SlirpProcess struct {
	addr      *net.UnixAddr
	listener  *net.UnixListener
	socketDir string
}

func (c *SlirpProcess) runConnection(helperPath string, conn *net.UnixConn) {
	connFile, err := conn.File()
	if err != nil {
		log.Printf("failed to get file for connection: %v", err)
		return
	}
	args := []string{
		helperPath,
		"--exit-with-parent",
		"--fd=3", // See Files[] array below
	}
	p, err := os.StartProcess(helperPath, args, &os.ProcAttr{
		Files: []*os.File{os.Stdin, os.Stdout, os.Stderr, connFile},
	})
	if err != nil {
		log.Printf("failed to start libslirp-helper subprocess: %v", err)
		return
	}
	ps, err := p.Wait()
	if err != nil {
		log.Printf("error waiting for libslirp-helper subprocess: %v", err)
		return
	}
	if ps.ExitCode() != 0 {
		log.Printf("libslirp-helper subprocess exited with error: %d", ps.ExitCode())
		return
	}
}

func (c *SlirpProcess) Start() error {
	var err error
	helperPath, err := exec.LookPath("libslirp-helper")
	if err != nil {
		return err
	}
	c.socketDir, err = os.MkdirTemp("", "ipxbox-libslirp-socket")
	if err != nil {
		return err
	}
	socketPath := filepath.Join(c.socketDir, "socket")
	c.addr, err = net.ResolveUnixAddr("unix", socketPath)
	if err != nil {
		return err
	}
	c.listener, err = net.ListenUnix("unix", c.addr)
	if err != nil {
		return err
	}

	go func() {
		for {
			conn, err := c.listener.AcceptUnix()
			if err != nil {
				log.Printf("terminating slirp listener, err=%v", err)
				break
			}
			go c.runConnection(helperPath, conn)
		}
	}()

	return nil
}

func (c *SlirpProcess) Close() error {
	return c.listener.Close()
}

func (c *SlirpProcess) CleanupSocketFiles() {
	os.RemoveAll(c.socketDir)
}

type SlirpConnection struct {
	conn *net.UnixConn
	buf  [1500]byte
}

func (c *SlirpProcess) Connect() (*SlirpConnection, error) {
	conn, err := net.DialUnix("unix", nil, c.addr)
	if err != nil {
		return nil, err
	}
	return &SlirpConnection{conn: conn}, nil
}

func (c *SlirpConnection) ReadPacketData() ([]byte, gopacket.CaptureInfo, error) {
	cnt, _, err := c.conn.ReadFromUnix(c.buf[:])
	if err != nil {
		return nil, gopacket.CaptureInfo{}, err
	}
	return c.buf[0:cnt], gopacket.CaptureInfo{}, nil
}

func (c *SlirpConnection) WritePacketData(buf []byte) error {
	_, err := c.conn.Write(buf)
	return err
}

func (c *SlirpConnection) Close() {
	c.conn.Close()
}

func MakeSlirp() (*SlirpConnection, error) {
	var proc SlirpProcess
	if err := proc.Start(); err != nil {
		return nil, err
	}
	conn, err := proc.Connect()
	proc.CleanupSocketFiles()
	if err != nil {
		proc.Close()
		return nil, err
	}
	return conn, nil
}

func openSlirp(arg string, captureNonIPX bool) (DuplexEthernetStream, error) {
	return MakeSlirp()
}
