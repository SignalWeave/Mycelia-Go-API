// mycelia_client.go
package mycelia

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/google/uuid"
)

// -----------------------------------------------------------------------------
// Constants (object and command types) — mirrors the Python code.
// -----------------------------------------------------------------------------

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
	API_PROTOCOL_VER uint8  = 1
	encodingName            = "utf-8"
	maxU16Len        uint32 = 65535
)

// -----------------------------------------------------------------------------
// Public message types (parallel to Python classes).
// -----------------------------------------------------------------------------

// Message sends a payload over a route.
type Message struct {
	SenderAddress string
	Route         string
	Payload       []byte
	// Optional: override, defaults to CMD_SEND if zero.
	CmdType uint8
}

func (m Message) cmdValid() bool { return m.effectiveCmd() == CMD_SEND }
func (m Message) effectiveCmd() uint8 {
	if m.CmdType != _CMD_UNKNOWN {
		return m.CmdType
	}
	return CMD_SEND
}

// Transformer registers/unregisters a transformer at a channel.
type Transformer struct {
	SenderAddress string
	Route         string
	Channel       string
	Address       string
	// Optional: override, defaults to CMD_ADD if zero.
	CmdType uint8
}

func (t Transformer) cmdValid() bool {
	c := t.effectiveCmd()
	return c == CMD_ADD || c == CMD_REMOVE
}
func (t Transformer) effectiveCmd() uint8 {
	if t.CmdType != _CMD_UNKNOWN {
		return t.CmdType
	}
	return CMD_ADD
}

// Subscriber registers/unregisters a subscriber at a channel.
type Subscriber struct {
	SenderAddress string
	Route         string
	Channel       string
	Address       string
	// Optional: override, defaults to CMD_ADD if zero.
	CmdType uint8
}

func (s Subscriber) cmdValid() bool {
	c := s.effectiveCmd()
	return c == CMD_ADD || c == CMD_REMOVE
}
func (s Subscriber) effectiveCmd() uint8 {
	if s.CmdType != _CMD_UNKNOWN {
		return s.CmdType
	}
	return CMD_ADD
}

// GlobalValues matches the Python shape. Only set fields you want to change.
type GlobalValues struct {
	Address          string // '' = ignore
	Port             int    // 0..65535 valid; others ignored
	Verbosity        int    // 0..3 valid; others ignored
	PrintTree        *bool  // nil = ignore
	TransformTimeout string // '' = ignore (e.g. "500ms")
}

// Globals updates broker globals.
type Globals struct {
	SenderAddress string
	Values        GlobalValues
	// Optional: override, defaults to CMD_UPDATE if zero.
	CmdType uint8
}

func (g Globals) cmdValid() bool { return g.effectiveCmd() == CMD_UPDATE }
func (g Globals) effectiveCmd() uint8 {
	if g.CmdType != _CMD_UNKNOWN {
		return g.CmdType
	}
	return CMD_UPDATE
}

// -----------------------------------------------------------------------------
// Encoding helpers (big-endian).
// -----------------------------------------------------------------------------

func putU8(buf *bytes.Buffer, n uint8) {
	_ = buf.WriteByte(n)
}

func putU16(buf *bytes.Buffer, n uint16) {
	var tmp [2]byte
	binary.BigEndian.PutUint16(tmp[:], n)
	buf.Write(tmp[:])
}

func putU32(buf *bytes.Buffer, n uint32) {
	var tmp [4]byte
	binary.BigEndian.PutUint32(tmp[:], n)
	buf.Write(tmp[:])
}

func pstr8(buf *bytes.Buffer, s string) error {
	b := []byte(s)
	if len(b) > 255 {
		return fmt.Errorf("string too long for u8 prefix: %d", len(b))
	}
	putU8(buf, uint8(len(b)))
	buf.Write(b)
	return nil
}

func pstr16(buf *bytes.Buffer, s string) error {
	b := []byte(s)
	if len(b) > int(maxU16Len) {
		return fmt.Errorf("string too long for u16 prefix: %d", len(b))
	}
	putU16(buf, uint16(len(b)))
	buf.Write(b)
	return nil
}

func pbytes16(buf *bytes.Buffer, b []byte) error {
	if len(b) > int(maxU16Len) {
		return fmt.Errorf("bytes too long for u16 prefix: %d", len(b))
	}
	putU16(buf, uint16(len(b)))
	buf.Write(b)
	return nil
}

// -----------------------------------------------------------------------------
// Frame builder (faithful to Python _encode_mycelia_obj).
// -----------------------------------------------------------------------------

// Encode builds the wire frame for any supported object.
// The frame layout is:
//
//	[u32 total_len] [u8 version][u8 obj][u8 cmd]
//	[u8 len uid][uid]
//	[u16 len sender][sender]
//	[u8 len arg1][u8 len arg2][u8 len arg3][u8 len arg4]
//	[u16 len payload][payload]
func Encode(obj any) ([]byte, error) {
	var (
		objType      uint8
		cmdType      uint8
		sender       string
		arg1, arg2   string
		arg3, arg4   string
		payloadBytes []byte
	)

	switch v := obj.(type) {
	case Message:
		objType = OBJ_MESSAGE
		cmdType = v.effectiveCmd()
		if !v.cmdValid() {
			return nil, errors.New("message: invalid cmd_type")
		}
		sender = v.SenderAddress
		arg1 = v.Route
		payloadBytes = append([]byte(nil), v.Payload...)

	case Transformer:
		objType = OBJ_TRANSFORMER
		cmdType = v.effectiveCmd()
		if !v.cmdValid() {
			return nil, errors.New("transformer: invalid cmd_type")
		}
		sender = v.SenderAddress
		arg1, arg2, arg3 = v.Route, v.Channel, v.Address
		// No payload for add/remove; will encode as zero-length.

	case Subscriber:
		objType = OBJ_SUBSCRIBER
		cmdType = v.effectiveCmd()
		if !v.cmdValid() {
			return nil, errors.New("subscriber: invalid cmd_type")
		}
		sender = v.SenderAddress
		arg1, arg2, arg3 = v.Route, v.Channel, v.Address

	case Globals:
		objType = OBJ_GLOBALS
		cmdType = v.effectiveCmd()
		if !v.cmdValid() {
			return nil, errors.New("globals: invalid cmd_type")
		}
		sender = v.SenderAddress
		// Build JSON map only for populated fields (like Python).
		data := make(map[string]any)
		if v.Values.Address != "" {
			data["address"] = v.Values.Address
		}
		if v.Values.Port > 0 && v.Values.Port < 65536 {
			data["port"] = v.Values.Port
		}
		if v.Values.Verbosity >= 0 && v.Values.Verbosity < 4 {
			data["verbosity"] = v.Values.Verbosity
		}
		if v.Values.PrintTree != nil {
			data["print_tree"] = *v.Values.PrintTree
		}
		if v.Values.TransformTimeout != "" {
			data["transform_timeout"] = v.Values.TransformTimeout
		}
		if len(data) == 0 {
			return nil, errors.New("globals: no valid fields to encode")
		}
		j, err := json.Marshal(data)
		if err != nil {
			return nil, fmt.Errorf("globals: marshal: %w", err)
		}
		payloadBytes = j

	default:
		return nil, fmt.Errorf("unsupported object type %T", obj)
	}

	if sender == "" {
		return nil, errors.New("sender address is required")
	}
	// Required args for message/subscriber/transformer.
	needsArgs := objType == OBJ_MESSAGE || objType == OBJ_SUBSCRIBER || objType == OBJ_TRANSFORMER
	if needsArgs && arg1 == "" {
		return nil, fmt.Errorf("object %d requires arg1", objType)
	}

	// Build the inner packet (without the u32 length prefix).
	body := bytes.NewBuffer(nil)

	// Fixed header
	putU8(body, API_PROTOCOL_VER)
	putU8(body, objType)
	putU8(body, cmdType)

	// Tracking sub-header
	_ = pstr8(body, uuid.NewString()) // u8-len UID
	if err := pstr16(body, sender); err != nil {
		return nil, err
	}

	// Args (four u8-len strings)
	if err := pstr8(body, arg1); err != nil {
		return nil, err
	}
	if err := pstr8(body, arg2); err != nil {
		return nil, err
	}
	if err := pstr8(body, arg3); err != nil {
		return nil, err
	}
	if err := pstr8(body, arg4); err != nil {
		return nil, err
	}

	// Payload (u16-len bytes)
	if err := pbytes16(body, payloadBytes); err != nil {
		return nil, err
	}

	// Prefix with total length (u32 big-endian).
	full := bytes.NewBuffer(nil)
	putU32(full, uint32(body.Len()))
	full.Write(body.Bytes())
	return full.Bytes(), nil
}

// Send connects to address:port and transmits the encoded frame.
func Send(obj any, address string, port int) error {
	frame, err := Encode(obj)
	if err != nil {
		return err
	}
	conn, err := net.Dial("tcp", net.JoinHostPort(address, strconv.Itoa(port)))
	if err != nil {
		return err
	}
	defer conn.Close()
	_, err = conn.Write(frame)
	return err
}

// -----------------------------------------------------------------------------
// Listener (blocking start/stop, like MyceliaListener in Python).
// -----------------------------------------------------------------------------

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
