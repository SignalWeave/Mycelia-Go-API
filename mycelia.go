package mycelia

import (
	"fmt"
	"net"
	"strings"

	"github.com/google/uuid"
)

const delimiter = ";;"

const (
	CMD_SEND_MESSAGE   = "send_message"
	CMD_ADD_SUBSCRIBER = "add_subscriber"
	CMD_ADD_CHANNEL    = "add_channel"
	CMD_ADD_ROUTE      = "add_route"
)

type CommandType interface {
	Serialize() string
}

// -------Send Message----------------------------------------------------------

type SendMessage struct {
	CmdType string
	ID      string
	Route   string
	Payload string
}

func NewSendMessage(route, payload string) *SendMessage {
	return &SendMessage{
		CmdType: CMD_SEND_MESSAGE,
		ID:      uuid.New().String(),
		Route:   route,
		Payload: payload,
	}
}

func (c *SendMessage) Serialize() string {
	return strings.Join([]string{c.CmdType, c.ID, c.Route, c.Payload}, delimiter)
}

// -------Add Subscriber--------------------------------------------------------

type AddSubscriber struct {
	CmdType string
	ID      string
	Route   string
	Channel string
	Address string
}

func NewAddSubscriber(route, channel, address string) *AddSubscriber {
	return &AddSubscriber{
		CmdType: CMD_ADD_SUBSCRIBER,
		ID:      uuid.New().String(),
		Route:   route,
		Channel: channel,
		Address: address,
	}
}

func (c *AddSubscriber) Serialize() string {
	return strings.Join([]string{c.CmdType, c.ID, c.Route, c.Channel, c.Address}, delimiter)
}

// -------Add Channel-----------------------------------------------------------

type AddChannel struct {
	CmdType string
	ID      string
	Route   string
	Name    string
}

func NewAddChannel(route, name string) *AddChannel {
	return &AddChannel{
		CmdType: CMD_ADD_CHANNEL,
		ID:      uuid.New().String(),
		Route:   route,
		Name:    name,
	}
}

func (c *AddChannel) Serialize() string {
	return strings.Join([]string{c.CmdType, c.ID, c.Route, c.Name}, delimiter)
}

// -------Add Router------------------------------------------------------------

type AddRoute struct {
	CmdType string
	ID      string
	Name    string
}

func NewAddRoute(name string) *AddRoute {
	return &AddRoute{
		CmdType: CMD_ADD_ROUTE,
		ID:      uuid.New().String(),
		Name:    name,
	}
}

func (c *AddRoute) Serialize() string {
	return strings.Join([]string{c.CmdType, c.ID, c.Name}, delimiter)
}

// -------Command Processing----------------------------------------------------

func formatAddress(host string, port int) string {
	if strings.Contains(host, ":") && !strings.HasPrefix(host, "[") {
		// Likely an IPv6 address
		return fmt.Sprintf("[%s]:%d", host, port)
	}
	return fmt.Sprintf("%s:%d", host, port)
}

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
