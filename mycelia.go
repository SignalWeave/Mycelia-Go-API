package mycelia

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"time"

	"github.com/google/uuid"
)

const (
	OBJ_MESSAGE     uint8 = 1
	OBJ_TRANSFORMER uint8 = 2
	OBJ_SUBSCRIBER  uint8 = 3
	OBJ_GLOBALS     uint8 = 4
)

const (
	_CMD_UNKNOWN uint8 = 0
	CMD_SEND     uint8 = 1
	CMD_ADD      uint8 = 2
	CMD_REMOVE   uint8 = 3
	CMD_UPDATE   uint8 = 4
)

const (
	API_PROTOCOL_VER uint8 = 1
)

// -------Types-----------------------------------------------------------------

// Base carries the common header fields.
type Base struct {
	ProtocolVersion uint8
	ObjType         uint8
	CmdType         uint8
	UID             string
}

type Routed struct {
	Route string
}

// Message = OBJ_MESSAGE (default CMD_SEND).
type Message struct {
	Base
	Routed
	Payload []byte
}

func NewMessage(route string, payload []byte) *Message {
	return &Message{
		Base: Base{
			ProtocolVersion: API_PROTOCOL_VER,
			ObjType:         OBJ_MESSAGE,
			CmdType:         CMD_SEND,
			UID:             "",
		},
		Routed: Routed{
			Route: route,
		},
		Payload: payload,
	}
}

// Transformer = OBJ_TRANSFORMER (default CMD_ADD).
type Transformer struct {
	Base
	Routed
	Channel string
	Address string
}

func NewTransformer(route, channel, address string) *Transformer {
	return &Transformer{
		Base: Base{
			ProtocolVersion: API_PROTOCOL_VER,
			ObjType:         OBJ_TRANSFORMER,
			CmdType:         CMD_ADD,
			UID:             "",
		},
		Routed: Routed{
			Route: route,
		},
		Channel: channel,
		Address: address,
	}
}

// Subscriber = OBJ_SUBSCRIBER (default CMD_ADD).
type Subscriber struct {
	Base
	Routed
	Channel string
	Address string
}

func NewSubscriber(route, channel, address string) *Subscriber {
	return &Subscriber{
		Base: Base{
			ProtocolVersion: API_PROTOCOL_VER,
			ObjType:         OBJ_SUBSCRIBER,
			CmdType:         CMD_ADD,
			UID:             "",
		},
		Routed: Routed{
			Route: route,
		},
		Channel: channel,
		Address: address,
	}
}

// Globals = OBJ_GLOBALS (default CMD_UPDATE)
type Globals struct {
	Base
	Address          string
	Port             int
	Verbosity        int8
	PrintTree        *bool
	TransformTimeout string
}

func NewGlobals() *Globals {
	return &Globals{
		Base: Base{
			ProtocolVersion: API_PROTOCOL_VER,
			ObjType:         OBJ_GLOBALS,
			CmdType:         CMD_UPDATE,
			UID:             "",
		},
		Address:          "",
		Port:             -1,
		Verbosity:        -1,
		PrintTree:        nil,
		TransformTimeout: "",
	}
}

// -------Encoding helpers (Big Endian, length-prefixed strings/bytes)----------

func u8(b *bytes.Buffer, v uint8) {
	// Endianness doesn't matter for 1 byte but Write() requires it.
	_ = binary.Write(b, binary.BigEndian, v)
}

func u32(b *bytes.Buffer, v uint32) {
	_ = binary.Write(b, binary.BigEndian, v)
}

func pstr(b *bytes.Buffer, s string) {
	u32(b, uint32(len(s)))
	_, _ = b.WriteString(s)
}

func pbytes(b *bytes.Buffer, p []byte) {
	u32(b, uint32(len(p)))
	_, _ = b.Write(p)
}

// -------Encoder --------------------------------------------------------------

// EncodeAny builds the protocol frame with the leading total length prefix.
func EncodeAny(v any) ([]byte, error) {
	var body bytes.Buffer

	switch t := v.(type) {
	case *Message:
		encodeMessage(t, body)
	case *Transformer:
		encodeTransformer(t, body)
	case *Subscriber:
		encodeSubscriber(t, body)
	case *Globals:
		err := encodeGlobals(t, body)
		if err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("EncodeAny: unsupported type %T", v)
	}

	// Prepend the total frame length
	packet := body.Bytes()
	var frame bytes.Buffer
	u32(&frame, uint32(len(packet)))
	_, _ = frame.Write(packet)

	return frame.Bytes(), nil
}

func encodeMessage(t *Message, body bytes.Buffer) {
	if t.Base.ProtocolVersion == 0 {
		t.Base.ProtocolVersion = API_PROTOCOL_VER
	}
	if t.Base.ObjType == 0 {
		t.Base.ObjType = OBJ_MESSAGE
	}
	if t.Base.CmdType == _CMD_UNKNOWN {
		t.Base.CmdType = CMD_SEND
	}
	if t.Base.UID == "" {
		t.Base.UID = uuid.NewString()
	}

	// Header
	u8(&body, t.Base.ProtocolVersion)
	u8(&body, t.Base.ObjType)
	u8(&body, t.Base.CmdType)
	pstr(&body, t.Base.UID)
	pstr(&body, t.Routed.Route)

	// Body
	pbytes(&body, t.Payload)
}

func encodeTransformer(t *Transformer, body bytes.Buffer) {
	if t.Base.ProtocolVersion == 0 {
		t.Base.ProtocolVersion = API_PROTOCOL_VER
	}
	if t.Base.ObjType == 0 {
		t.Base.ObjType = OBJ_TRANSFORMER
	}
	if t.Base.CmdType == _CMD_UNKNOWN {
		t.Base.CmdType = CMD_ADD
	}
	if t.Base.UID == "" {
		t.Base.UID = uuid.NewString()
	}

	// Header
	u8(&body, t.Base.ProtocolVersion)
	u8(&body, t.Base.ObjType)
	u8(&body, t.Base.CmdType)
	pstr(&body, t.Base.UID)
	pstr(&body, t.Routed.Route)

	// Conditional Header
	pstr(&body, t.Channel)
	pstr(&body, t.Address)
}

func encodeSubscriber(t *Subscriber, body bytes.Buffer) {
	if t.Base.ProtocolVersion == 0 {
		t.Base.ProtocolVersion = API_PROTOCOL_VER
	}
	if t.Base.ObjType == 0 {
		t.Base.ObjType = OBJ_SUBSCRIBER
	}
	if t.Base.CmdType == _CMD_UNKNOWN {
		t.Base.CmdType = CMD_ADD
	}
	if t.Base.UID == "" {
		t.Base.UID = uuid.NewString()
	}

	// Header
	u8(&body, t.Base.ProtocolVersion)
	u8(&body, t.Base.ObjType)
	u8(&body, t.Base.CmdType)
	pstr(&body, t.Base.UID)
	pstr(&body, t.Routed.Route)

	// Conditional Header
	pstr(&body, t.Channel)
	pstr(&body, t.Address)
}

func encodeGlobals(t *Globals, body bytes.Buffer) error {
	if t.Base.ProtocolVersion == 0 {
		t.Base.ProtocolVersion = API_PROTOCOL_VER
	}
	if t.Base.ObjType == 0 {
		t.Base.ObjType = OBJ_SUBSCRIBER
	}
	if t.Base.CmdType == _CMD_UNKNOWN {
		t.Base.CmdType = CMD_UPDATE
	}
	if t.Base.UID == "" {
		t.Base.UID = uuid.NewString()
	}

	values := map[string]any{}

	if t.Address != "" {
		values["address"] = t.Address
	}
	if t.Port > 0 && t.Port < 65536 {
		values["port"] = uint8(t.Port)
	}
	if t.Verbosity > -1 && t.Verbosity < 4 {
		values["verbosity"] = uint8(t.Verbosity)
	}
	if t.PrintTree != nil {
		values["print_tree"] = *t.PrintTree
	}
	if t.TransformTimeout != "" {
		values["transform_timeout"] = t.TransformTimeout
	}

	b, err := json.Marshal(values)
	if err != nil {
		return err
	}
	valueString := string(b)

	pstr(&body, valueString)
	return nil
}

// -------Network Boilerplate---------------------------------------------------

func ProcessCommand(v interface{}, address string, port int) error {
	frame, err := EncodeAny(v)
	if err != nil {
		return err
	}

	conn, err := net.Dial("tcp", net.JoinHostPort(address, strconv.Itoa(port)))
	if err != nil {
		return err
	}
	defer conn.Close()

	// Ensure full send.
	_, err = conn.Write(frame)
	return err
}

// GetLocalIPv4 returns the primary local IPv4, or 127.0.0.1 on failure.
func GetLocalIPv4() string {
	// Trick: connect UDP to a non-routable address, then read the local addr.
	conn, err := net.Dial("udp", "10.255.255.255:1")
	if err != nil {
		return "127.0.0.1"
	}
	defer conn.Close()
	local := conn.LocalAddr()
	udpAddr, ok := local.(*net.UDPAddr)
	if !ok || udpAddr.IP == nil {
		return "127.0.0.1"
	}
	return udpAddr.IP.String()
}

type MyceliaListener struct {
	LocalAddr string
	LocalPort int
	// If the processor returns (reply, true), the reply is sent back to the
	// client, if it returns (_, false), no reply is sent.
	MessageProcessor func([]byte) ([]byte, bool)

	stop chan struct{}
	ln   *net.TCPListener
}

// NewMyceliaListener constructs a listener.
func NewMyceliaListener(
	proc func([]byte) ([]byte, bool),
	localAddr string,
	localPort int,
) *MyceliaListener {
	if localAddr == "" {
		localAddr = GetLocalIPv4()
	}
	if localPort == 0 {
		localPort = 5500
	}
	return &MyceliaListener{
		LocalAddr:        localAddr,
		LocalPort:        localPort,
		MessageProcessor: proc,
		stop:             make(chan struct{}),
	}
}

// Start blocks, accepting connections and forwarding received chunks
// to MessageProcessor; if it returns (reply, true) we write the reply back.
func (l *MyceliaListener) Start() error {
	addr := fmt.Sprintf("%s:%d", l.LocalAddr, l.LocalPort)
	rawLn, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	tcpLn := rawLn.(*net.TCPListener)
	l.ln = tcpLn

	fmt.Printf("Listening on %s\n", addr)

	for {
		// Make Accept interruptible via deadline so we can check stop signal.
		_ = tcpLn.SetDeadline(time.Now().Add(1 * time.Second))

		conn, err := tcpLn.Accept()
		if err != nil {
			// Check if deadline or closed
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				select {
				case <-l.stop:
					return nil
				default:
					continue
				}
			}
			// If listener was closed, exit gracefully.
			select {
			case <-l.stop:
				return nil
			default:
				// Unexpected error
				return err
			}
		}

		go func(c net.Conn) {
			defer c.Close()
			fmt.Printf("Connected by %s\n", c.RemoteAddr())

			// Allocate a buffer per-connection to avoid races across
			// goroutines.
			buf := make([]byte, 1024)

			for {
				n, rerr := c.Read(buf)
				if n > 0 && l.MessageProcessor != nil {
					reply, ok := l.MessageProcessor(buf[:n])
					if ok {
						// Note: writing zero bytes is a no-op, which is fine.
						if _, werr := c.Write(reply); werr != nil {
							return
						}
					}
				}
				if rerr != nil {
					if errors.Is(rerr, io.EOF) {
						return
					}
					// Any other read error ends the connection loop
					return
				}
			}
		}(conn)
	}
}

// Stop signals the listener to stop and closes the socket.
func (l *MyceliaListener) Stop() {
	select {
	case <-l.stop:
		// already stopped
	default:
		close(l.stop)
	}
	if l.ln != nil {
		_ = l.ln.Close()
	}
}
