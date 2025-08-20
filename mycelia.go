package mycelia

import (
	"fmt"
	"net"
	"strings"

	"github.com/google/uuid"
)

const delimiter = ";;"

const (
	CMD_SEND_MESSAGE    = "send_message"
	CMD_ADD_SUBSCRIBER  = "add_subscriber"
	CMD_ADD_CHANNEL     = "add_channel"
	CMD_ADD_ROUTE       = "add_route"
	CMD_ADD_TRANSFORMER = "add_transformer"
)

// The integer version number of which protocol version the remote broker should
// use to decode data fields.
// Can be set by user if needed.
var ProtocolVerison = "1"

type CommandType interface {
	Serialize() string
}

// -------Send Message----------------------------------------------------------

type SendMessage struct {
	ProtoVer string
	CmdType  string
	ID       string
	Route    string
	Payload  string
}

func NewSendMessage(route, payload string) *SendMessage {
	return &SendMessage{
		ProtoVer: ProtocolVerison,
		CmdType:  CMD_SEND_MESSAGE,
		ID:       uuid.New().String(),
		Route:    route,
		Payload:  payload,
	}
}

func (c *SendMessage) Serialize() string {
	return strings.Join(
		[]string{c.ProtoVer, c.CmdType, c.ID, c.Route, c.Payload}, delimiter,
	)
}

// -------Add Subscriber--------------------------------------------------------

type AddSubscriber struct {
	ProtoVer string
	CmdType  string
	ID       string
	Route    string
	Channel  string
	Address  string
}

func NewAddSubscriber(route, channel, address string) *AddSubscriber {
	return &AddSubscriber{
		ProtoVer: ProtocolVerison,
		CmdType:  CMD_ADD_SUBSCRIBER,
		ID:       uuid.New().String(),
		Route:    route,
		Channel:  channel,
		Address:  address,
	}
}

func (c *AddSubscriber) Serialize() string {
	return strings.Join(
		[]string{c.ProtoVer, c.CmdType, c.ID, c.Route, c.Channel, c.Address},
		delimiter,
	)
}

// -------Add Channel-----------------------------------------------------------

type AddChannel struct {
	ProtoVer string
	CmdType  string
	ID       string
	Route    string
	Name     string
}

func NewAddChannel(route, name string) *AddChannel {
	return &AddChannel{
		ProtoVer: ProtocolVerison,
		CmdType:  CMD_ADD_CHANNEL,
		ID:       uuid.New().String(),
		Route:    route,
		Name:     name,
	}
}

func (c *AddChannel) Serialize() string {
	return strings.Join([]string{c.ProtoVer, c.CmdType, c.ID, c.Route, c.Name},
		delimiter,
	)
}

// -------Add Route-------------------------------------------------------------

type AddRoute struct {
	ProtoVer string
	CmdType  string
	ID       string
	Name     string
}

func NewAddRoute(name string) *AddRoute {
	return &AddRoute{
		ProtoVer: ProtocolVerison,
		CmdType:  CMD_ADD_ROUTE,
		ID:       uuid.New().String(),
		Name:     name,
	}
}

func (c *AddRoute) Serialize() string {
	return strings.Join([]string{c.ProtoVer, c.CmdType, c.ID, c.Name},
		delimiter,
	)
}

// -------Add Transformer-------------------------------------------------------

type AddTransformer struct {
	ProtoVer string
	CmdType  string
	ID       string
	Route    string
	Channel  string
	Address  string
}

func NewAddTransformer(route, channel, address string) *AddTransformer {
	return &AddTransformer{
		ProtoVer: ProtocolVerison,
		CmdType:  CMD_ADD_TRANSFORMER,
		ID:       uuid.New().String(),
		Route:    route,
		Channel:  channel,
		Address:  address,
	}
}

func (c *AddTransformer) Serialize() string {
	return strings.Join(
		[]string{c.ProtoVer, c.CmdType, c.ID, c.Route, c.Channel, c.Address},
		delimiter,
	)
}

// -------Command Processing----------------------------------------------------

// Formats the address for friendly IPv6 or IPv4 based on contained characterse.
func formatAddress(host string, port int) string {
	if strings.Contains(host, ":") && !strings.HasPrefix(host, "[") {
		// Likely an IPv6 address
		return fmt.Sprintf("[%s]:%d", host, port)
	}
	return fmt.Sprintf("%s:%d", host, port)
}

// Sends the command to the Mycelia client at the given address + port.
func ProcessCommand(cmd CommandType, address string, port int) error {
	payload := cmd.Serialize()
	fullAddr := formatAddress(address, port)
	conn, err := net.Dial("tcp", fullAddr)
	if err != nil {
		return err
	}
	defer conn.Close()

	_, err = conn.Write([]byte(payload))
	return err
}
