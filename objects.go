package mycelia

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"slices"
	"strconv"

	"github.com/google/uuid"
)

const (
	OBJ_MESSAGE     uint8 = 1
	OBJ_TRANSFORMER uint8 = 2
	OBJ_SUBSCRIBER  uint8 = 3
	OBJ_CHANNEL     uint8 = 4

	OBJ_GLOBALS uint8 = 20

	OBJ_ACTION uint8 = 50
)

const (
	_CMD_UNKNOWN uint8 = 0

	CMD_SEND   uint8 = 1
	CMD_ADD    uint8 = 2
	CMD_REMOVE uint8 = 3

	CMD_UPDATE uint8 = 20

	CMD_SIGTERM uint8 = 50
)

const (
	API_PROTOCOL_VER uint8  = 1
	encodingName            = "utf-8"
	maxU16Len        uint32 = 65535
)

// DEAD_LETTER is used for subscribing to dead letter channels.
const DEAD_LETTER = "deadLetter"

// -------Public message types--------------------------------------------------

type Command interface {
	CmdValid() bool
	EffectiveCmd() uint8
}

// Message sends a payload over a route.
type Message struct {
	AckPolicy ACK_PLCY
	Route     string
	Payload   []byte
	// Optional: override, defaults to CMD_SEND if zero.
	CmdType uint8
}

func (m Message) CmdValid() bool {
	c := m.EffectiveCmd()
	return c == CMD_SEND
}

func (m Message) EffectiveCmd() uint8 {
	if m.CmdType != _CMD_UNKNOWN {
		return m.CmdType
	}
	return CMD_SEND
}

// Transformer registers/unregisters a transformer at a channel.
type Transformer struct {
	AckPolicy ACK_PLCY
	Route     string
	Channel   string
	Address   string
	// Optional: override, defaults to CMD_ADD if zero.
	CmdType uint8
}

func (t Transformer) CmdValid() bool {
	c := t.EffectiveCmd()
	return c == CMD_ADD || c == CMD_REMOVE
}
func (t Transformer) EffectiveCmd() uint8 {
	if t.CmdType != _CMD_UNKNOWN {
		return t.CmdType
	}
	return CMD_ADD
}

// Subscriber registers/unregisters a subscriber at a channel.
type Subscriber struct {
	AckPolicy ACK_PLCY
	Route     string
	Channel   string
	Address   string
	// Optional: override, defaults to CMD_ADD if zero.
	CmdType uint8
}

func (s Subscriber) CmdValid() bool {
	c := s.EffectiveCmd()
	return c == CMD_ADD || c == CMD_REMOVE
}
func (s Subscriber) EffectiveCmd() uint8 {
	if s.CmdType != _CMD_UNKNOWN {
		return s.CmdType
	}
	return CMD_ADD
}

type GlobalValues struct {
	SecurityToken    string
	Address          string // '' = ignore
	Port             int    // 0..65535 valid; others ignored
	Verbosity        int    // 0..3 valid; others ignored
	PrintTree        *bool  // nil = ignore
	TransformTimeout string // '' = ignore (e.g. "500ms")
	Consolidate      *bool  // nil = ignore
}

// Globals updates broker globals.
type Globals struct {
	AckPolicy ACK_PLCY
	Values    GlobalValues
	// Optional: override, defaults to CMD_UPDATE if zero.
	CmdType uint8
}

func (g Globals) CmdValid() bool {
	c := g.EffectiveCmd()
	return c == CMD_UPDATE
}

func (g Globals) EffectiveCmd() uint8 {
	if g.CmdType != _CMD_UNKNOWN {
		return g.CmdType
	}
	return CMD_UPDATE
}

type Channel struct {
	AckPolicy ACK_PLCY
	Route     string
	Name      string
	// Optional: override, defaults to SEL_STRAT_PUBSUB if zero.
	SelectionStrategy SEL_STRAT
	// Optional: override, defaults to CMD_ADD if zero.
	CmdType uint8
}

func (c Channel) CmdValid() bool {
	cmd := c.EffectiveCmd()
	return cmd == CMD_ADD || cmd == CMD_REMOVE
}

func (c Channel) EffectiveCmd() uint8 {
	if c.CmdType != _CMD_UNKNOWN {
		return c.CmdType
	}
	return CMD_ADD
}

// Action invokes application level commands of the broker.
type Action struct {
	AckPolicy ACK_PLCY
	// Optional: override, defaults to CMD_SIGTERM if zero.
	CmdType uint8
}

func (a Action) CmdValid() bool {
	c := a.EffectiveCmd()
	return c == CMD_SIGTERM
}
func (a Action) EffectiveCmd() uint8 {
	if a.CmdType != _CMD_UNKNOWN {
		return a.CmdType
	}
	return CMD_SIGTERM
}

// -------Encoding helpers (big-endian)-----------------------------------------

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

func pbytes16(buf *bytes.Buffer, b []byte) error {
	if len(b) > int(maxU16Len) {
		return fmt.Errorf("bytes too long for u16 prefix: %d", len(b))
	}
	putU16(buf, uint16(len(b)))
	buf.Write(b)
	return nil
}

// -------Frame builder---------------------------------------------------------

type frame struct {
	objType      uint8
	cmdType      uint8
	ackPlcy      uint8
	arg1, arg2   string
	arg3, arg4   string
	payloadBytes []byte
}

func encodeMessage(msg Message) (*frame, error) {
	f := &frame{}
	f.objType = OBJ_MESSAGE
	f.cmdType = msg.EffectiveCmd()
	if !msg.CmdValid() {
		return nil, errors.New("message: invalid cmd_type")
	}
	f.ackPlcy = uint8(msg.AckPolicy)

	f.arg1 = msg.Route
	f.arg2 = ""
	f.arg3 = ""
	f.arg4 = ""

	f.payloadBytes = append([]byte(nil), msg.Payload...)

	return f, nil
}

func encodeTransformer(tfr Transformer) (*frame, error) {
	f := &frame{}
	f.objType = OBJ_TRANSFORMER
	f.cmdType = tfr.EffectiveCmd()
	if !tfr.CmdValid() {
		return nil, errors.New("transformer: invalid cmd_type")
	}
	f.ackPlcy = uint8(tfr.AckPolicy)

	f.arg1, f.arg2, f.arg3, f.arg4 = tfr.Route, tfr.Channel, tfr.Address, ""

	return f, nil
}

func encodeSubscriber(sub Subscriber) (*frame, error) {
	f := &frame{}
	f.objType = OBJ_SUBSCRIBER
	f.cmdType = sub.EffectiveCmd()
	if !sub.CmdValid() {
		return nil, errors.New("subscriber: invalid cmd_type")
	}
	f.ackPlcy = uint8(sub.AckPolicy)

	f.arg1, f.arg2, f.arg3, f.arg4 = sub.Route, sub.Channel, sub.Address, ""

	return f, nil
}

func encodeGlobals(glb Globals) (*frame, error) {
	f := &frame{}
	f.objType = OBJ_GLOBALS
	f.cmdType = glb.EffectiveCmd()
	if !glb.CmdValid() {
		return nil, errors.New("globals: invalid cmd_type")
	}
	f.ackPlcy = uint8(glb.AckPolicy)

	f.arg1, f.arg2, f.arg3, f.arg4 = "", "", "", ""

	data := make(map[string]any)
	if glb.Values.Address != "" {
		data["address"] = glb.Values.Address
	}
	if glb.Values.Port > 0 && glb.Values.Port < 65536 {
		data["port"] = glb.Values.Port
	}
	if glb.Values.Verbosity >= 0 && glb.Values.Verbosity < 4 {
		data["verbosity"] = glb.Values.Verbosity
	}
	if glb.Values.PrintTree != nil {
		data["print_tree"] = *glb.Values.PrintTree
	}
	if glb.Values.TransformTimeout != "" {
		data["transform_timeout"] = glb.Values.TransformTimeout
	}
	if glb.Values.Consolidate != nil {
		data["consolidate"] = *glb.Values.Consolidate
	}
	if glb.Values.SecurityToken != "" {
		return nil, errors.New("globals: security token required")
	}
	if len(data) == 0 {
		return nil, errors.New("globals: no valid fields to encode")
	}
	j, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("globals: marshal: %w", err)
	}
	f.payloadBytes = j

	return f, nil
}

func encodeChannel(ch Channel) (*frame, error) {
	f := &frame{}
	f.objType = OBJ_CHANNEL
	f.cmdType = ch.EffectiveCmd()
	if !ch.CmdValid() {
		return nil, errors.New("channel: invalid cmd_type")
	}
	f.ackPlcy = uint8(ch.AckPolicy)

	f.arg1, f.arg2 = ch.Route, ch.Name
	f.arg3, f.arg4 = ch.SelectionStrategy.String(), ""

	return f, nil
}

func encodeAction(act Action) (*frame, error) {
	f := &frame{}
	f.objType = OBJ_ACTION
	f.cmdType = act.EffectiveCmd()
	if !act.CmdValid() {
		return nil, errors.New("action: invalid cmd_type")
	}
	f.ackPlcy = uint8(act.AckPolicy)

	f.arg1, f.arg2, f.arg3, f.arg4 = "", "", "", ""

	return f, nil
}

func encode(cmd Command) ([]byte, error) {
	var f *frame
	var err error

	switch v := cmd.(type) {
	case Message:
		f, err = encodeMessage(v)
		if err != nil {
			return nil, err
		}
	case Transformer:
		f, err = encodeTransformer(v)
		if err != nil {
			return nil, err
		}
	case Subscriber:
		f, err = encodeSubscriber(v)
		if err != nil {
			return nil, err
		}
	case Globals:
		f, err = encodeGlobals(v)
		if err != nil {
			return nil, err
		}
	case Channel:
		f, err = encodeChannel(v)
		if err != nil {
			return nil, err
		}
	case Action:
		f, err = encodeAction(v)
		if err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unsupported object type %T", cmd)
	}

	b, err := encodeFrame(f)
	if err != nil {
		return nil, err
	}

	return b, nil
}

func encodeFrame(f *frame) ([]byte, error) {
	body := bytes.NewBuffer(nil)

	// -----Fixed header-----
	putU8(body, API_PROTOCOL_VER)
	putU8(body, f.objType)
	putU8(body, f.cmdType)

	// -----Tracking sub-header-----
	_ = pstr8(body, uuid.NewString())

	// -----Arguments-----
	needsArgs := []uint8{OBJ_MESSAGE, OBJ_SUBSCRIBER, OBJ_TRANSFORMER}
	if slices.Contains(needsArgs, f.objType) && f.arg1 == "" {
		return nil, errors.New("message has incomplete args")
	}

	if err := pstr8(body, f.arg1); err != nil {
		return nil, err
	}
	if err := pstr8(body, f.arg2); err != nil {
		return nil, err
	}
	if err := pstr8(body, f.arg3); err != nil {
		return nil, err
	}
	if err := pstr8(body, f.arg4); err != nil {
		return nil, err
	}

	// -----Payload-----
	if err := pbytes16(body, f.payloadBytes); err != nil {
		return nil, err
	}

	// Prefix with total length (u32 big-endian).
	full := bytes.NewBuffer(nil)
	putU32(full, uint32(body.Len()))
	full.Write(body.Bytes())
	return full.Bytes(), nil
}

// Send connects to address:port and transmits the encoded frame.
// Returns *Response or error.
func Send(cmd Command, address string, port int) (*Response, error) {
	frame, err := encode(cmd)
	if err != nil {
		return nil, err
	}

	conn, err := net.Dial("tcp", net.JoinHostPort(address, strconv.Itoa(port)))
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	_, err = conn.Write(frame)
	if err != nil {
		return nil, err
	}

	response, err := recvAndDecode(conn)
	if err != nil {
		return nil, err
	}

	return response, nil
}
