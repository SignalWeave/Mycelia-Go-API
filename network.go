package mycelia

import (
	"errors"
	"fmt"
	"net"
	"time"
)

// Processor returns an optional response. If nil, no response is sent.
type Processor func(payload []byte) []byte

type MyceliaListener struct {
	localAddr string
	localPort int
	process   Processor

	ln   net.Listener
	stop chan struct{}
}

func NewMyceliaListener(
	processor Processor,
	localAddr string,
	localPort int) *MyceliaListener {
	return &MyceliaListener{
		localAddr: localAddr,
		localPort: localPort,
		process:   processor,
		stop:      make(chan struct{}),
	}
}

// Start blocks, accepting and handling connections until Stop is called.
func (ml *MyceliaListener) Start() error {
	ln, err := net.Listen("tcp", fmt.Sprintf("%s:%d", ml.localAddr, ml.localPort))
	if err != nil {
		return err
	}
	ml.ln = ln
	defer ln.Close()

	for {
		// Use deadline to periodically check stop channel.
		if tcpln, ok := ln.(*net.TCPListener); ok {
			_ = tcpln.SetDeadline(time.Now().Add(1 * time.Second))
		}
		conn, err := ln.Accept()
		select {
		case <-ml.stop:
			return nil
		default:
		}
		if err != nil {
			// Deadline or closed listener?
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				continue
			}
			// If Stop closed the listener, exit cleanly.
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			// Other transient accept errors: continue.
			continue
		}
		go ml.handleConn(conn)
	}
}

func (ml *MyceliaListener) handleConn(conn net.Conn) {
	defer conn.Close()
	buf := make([]byte, 1024)
	for {
		_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		n, err := conn.Read(buf)
		if n == 0 && err != nil {
			return
		}
		payload := append([]byte(nil), buf[:n]...)
		reply := ml.process(payload)
		if reply != nil {
			_, _ = conn.Write(reply)
		}
	}
}

// Stop unblocks Start by closing the listener.
func (ml *MyceliaListener) Stop() {
	select {
	case <-ml.stop:
		// already stopped
	default:
		close(ml.stop)
	}
	if ml.ln != nil {
		_ = ml.ln.Close()
	}
}

// GetLocalIPv4 returns the primary outbound IPv4 (fallback 127.0.0.1).
func GetLocalIPv4() string {
	conn, err := net.Dial("udp", "10.255.255.255:1")
	if err != nil {
		return "127.0.0.1"
	}
	defer conn.Close()
	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String()
}
