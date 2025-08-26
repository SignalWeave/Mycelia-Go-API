package mycelia

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"time"

	"github.com/google/uuid"
)

const (
	// The integer version number of which protocol version the remote broker
	// should use to decode data fields.
	API_VERSION = 1

	// Version 1 of the command API does not support sub-command parsing.
	// The <object>.<action> syntax is the conform to future version feature
	// syntax.
	CMD_SEND_MESSAGE    = "MESSAGE.ADD"
	CMD_ADD_TRANSFORMER = "TRANSFORMER.ADD"
	CMD_ADD_SUBSCRIBER  = "SUBSCRIBER.ADD"
)

type CommandType interface {
	SerializeFields() []string
}

// -------Send Message----------------------------------------------------------

// SendMessage sends a payload through a route.
type SendMessage struct {
	ProtoVer string
	CmdType  string
	ID       string
	Route    string
	Payload  string
}

func NewSendMessage(route, payload string, protoVer int) *SendMessage {
	return &SendMessage{
		ProtoVer: strconv.Itoa(protoVer),
		CmdType:  CMD_SEND_MESSAGE,
		ID:       uuid.NewString(),
		Route:    route,
		Payload:  payload,
	}
}

func (c *SendMessage) SerializeFields() []string {
	return []string{c.ProtoVer, c.CmdType, c.ID, c.Route, c.Payload}
}

// -------Add Subscriber--------------------------------------------------------

// AddSubscriber registers a subscriber at address on route+channel.
type AddSubscriber struct {
	ProtoVer string
	CmdType  string
	ID       string
	Route    string
	Channel  string
	Address  string
}

func NewAddSubscriber(
	route,
	channel,
	address string,
	protoVer int,
) *AddSubscriber {
	return &AddSubscriber{
		ProtoVer: strconv.Itoa(protoVer),
		CmdType:  CMD_ADD_SUBSCRIBER,
		ID:       uuid.NewString(),
		Route:    route,
		Channel:  channel,
		Address:  address,
	}
}

func (c *AddSubscriber) SerializeFields() []string {
	return []string{c.ProtoVer, c.CmdType, c.ID, c.Route, c.Channel, c.Address}
}

// -------Add Transformer-------------------------------------------------------

// AddTransformer registers a transformer on route+channel to address.
type AddTransformer struct {
	ProtoVer string
	CmdType  string
	ID       string
	Route    string
	Channel  string
	Address  string
}

func NewAddTransformer(
	route,
	channel,
	address string,
	protoVer int,
) *AddTransformer {
	return &AddTransformer{
		ProtoVer: strconv.Itoa(protoVer),
		CmdType:  CMD_ADD_TRANSFORMER,
		ID:       uuid.NewString(),
		Route:    route,
		Channel:  channel,
		Address:  address,
	}
}

func (c *AddTransformer) SerializeFields() []string {
	return []string{c.ProtoVer, c.CmdType, c.ID, c.Route, c.Channel, c.Address}
}

// -------Command Processing----------------------------------------------------

// encodeUvarint encodes n using unsigned LEB128.
func encodeUvarint(n uint64) []byte {
	var buf [binary.MaxVarintLen64]byte
	k := binary.PutUvarint(buf[:], n)
	return buf[:k]
}

// serializeMessage encodes fields as [uvarint length][bytes]...
func serializeMessage(cmd CommandType) []byte {
	var b bytes.Buffer
	for _, f := range cmd.SerializeFields() {
		if f == "" {
			continue
		}
		fb := []byte(f) // Go strings are UTF-8
		b.Write(encodeUvarint(uint64(len(fb))))
		b.Write(fb)
	}
	return b.Bytes()
}

// ProcessCommand connects to address:port and sends [uvarint msgLen][payload].
func ProcessCommand(cmd CommandType, address string, port int) error {
	payload := serializeMessage(cmd)
	frame := append(encodeUvarint(uint64(len(payload))), payload...)

	conn, err := net.Dial("tcp", net.JoinHostPort(address, strconv.Itoa(port)))
	if err != nil {
		return err
	}
	defer conn.Close()

	_, err = conn.Write(frame)
	return err
}

// --------Network Boilerplate--------------------------------------------------

// GetLocalIPv4 returns the host's primary IPv4, or 127.0.0.1 on failure.
// It mirrors the Python trick of UDP "connect" to discover the chosen interface.
func GetLocalIPv4() string {
	conn, err := net.DialTimeout("udp", "10.255.255.255:1", 2*time.Second)
	if err != nil {
		return "127.0.0.1"
	}
	defer conn.Close()
	if la, ok := conn.LocalAddr().(*net.UDPAddr); ok && la.IP != nil {
		return la.IP.String()
	}
	return "127.0.0.1"
}

// MyceliaListener binds a TCP port and forwards incoming framed payloads
// to MessageProcessor. It reads [uvarint msgLen][payload] repeatedly.
type MyceliaListener struct {
	LocalAddr        string
	LocalPort        int
	MessageProcessor func([]byte)

	ln     net.Listener
	closed chan struct{}
}

func NewMyceliaListener(
	localAddr string,
	localPort int,
	handler func([]byte),
) *MyceliaListener {
	return &MyceliaListener{
		LocalAddr:        localAddr,
		LocalPort:        localPort,
		MessageProcessor: handler,
		closed:           make(chan struct{}),
	}
}

// Start runs accept loop (blocking). Call Stop from another goroutine to exit.
func (m *MyceliaListener) Start() error {
	addr := net.JoinHostPort(m.LocalAddr, strconv.Itoa(m.LocalPort))
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	m.ln = ln
	fmt.Printf("Listening on %s\n", addr)

	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-m.closed:
				return nil // closed by Stop
			default:
				return err
			}
		}
		go m.handleConn(conn)
	}
}

// Stop closes the listener socket.
func (m *MyceliaListener) Stop() {
	close(m.closed)
	if m.ln != nil {
		_ = m.ln.Close()
	}
}

func (m *MyceliaListener) handleConn(conn net.Conn) {
	defer conn.Close()
	fmt.Printf("Connected by %s\n", conn.RemoteAddr().String())

	reader := bufio.NewReader(conn)
	for {
		msgLen, err := binary.ReadUvarint(reader)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return
			}
			// partial/invalid frame ends this connection
			fmt.Printf("Bad message length: %v\n", err)
			return
		}
		if msgLen == 0 {
			continue
		}

		msg := make([]byte, msgLen)
		if _, err := io.ReadFull(reader, msg); err != nil {
			if errors.Is(err, io.EOF) {
				return
			}
			fmt.Printf("Bad message body: %v\n", err)
			return
		}

		// Hand off the raw payload bytes
		if m.MessageProcessor != nil {
			m.MessageProcessor(msg)
		}
	}
}
