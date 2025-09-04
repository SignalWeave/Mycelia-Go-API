package mycelia

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
)

// -------Ack values------------------------------------------------------------

// How the sender would like to be informed about their message by the broker.
// No reply, was it forwarded, etc.
type ACK_PLCY uint8

const (
	// Sender does not wish to receive ack.
	ACK_PLCY_NOREPLY ACK_PLCY = 0

	// Sender wants to get ack when broker delivers to final subscriber.
	// This often means sending the ack back after the final channel has
	// processed the message object.
	ACK_PLCY_ONSENT ACK_PLCY = 1
)

var ackPolicyName = map[ACK_PLCY]string{
	ACK_PLCY_NOREPLY: "NoReply",
	ACK_PLCY_ONSENT:  "OnSent",
}

func (ap ACK_PLCY) String() string {
	return ackPolicyName[ap]
}

// The response code from the broker.
// Sent, timed out, etc.
type ACK_TYPE uint8

const (
	ACK_TYPE_UNKNOWN ACK_TYPE = 0 // Undetermined

	// Broker was able to and finished sending message to subscribers.
	ACK_TYPE_SENT ACK_TYPE = 1

	// If no ack was gotten before the timeout time, a response with ACK_TIMEOUT
	// is generated and returned instead.
	ACK_TYPE_TIMEOUT ACK_TYPE = 10
)

var ackTypeName = map[ACK_TYPE]string{
	ACK_TYPE_UNKNOWN: "Unknown",
	ACK_TYPE_SENT:    "Sent",
	ACK_TYPE_TIMEOUT: "Timeout",
}

func (at ACK_TYPE) String() string {
	return ackTypeName[at]
}

// The response from the broker with a given ack code and the corresponding
// message's UID.
type Response struct {
	UID string
	ACK ACK_TYPE
}

// -------Decoding--------------------------------------------------------------

var (
	ErrShortBody      = errors.New("body too short to contain uidLen and ack")
	ErrLengthMismatch = errors.New("body length does not match uidLen+2")
	ErrUIDOverflow    = errors.New("uidLen exceeds body size")
)

// recvAndDecode reads a single message from conn and decodes it according to
// the spec.
// Layout:
//
//	[0..1]   uint16 bodyLen (big-endian)
//	[2]      uint8  uidLen
//	[...]    uid bytes (len = uidLen)
//	[last]   uint8  ack
func recvAndDecode(conn net.Conn) (*Response, error) {
	var hdr [2]byte
	if _, err := io.ReadFull(conn, hdr[:]); err != nil {
		return nil, fmt.Errorf("read body length: %w", err)
	}

	bodyLen := binary.BigEndian.Uint16(hdr[:])
	if bodyLen == 0 {
		return nil, fmt.Errorf("invalid body length: %d", bodyLen)
	}

	body := make([]byte, int(bodyLen))

	// A message could contain an empty uid string so make sure it atleast has
	// the u8 uid len hdr and u8 ack type.
	if len(body) < 2 {
		return nil, ErrShortBody
	}
	uidLen := int(body[0])

	// Validate declared uidLen fits in the body:
	// need 1(uidLen) + uidLen + 1(ack)
	if uidLen < 0 || 1+uidLen+1 > len(body) {
		return nil, ErrUIDOverflow
	}
	// Exact match to the announced bodyLen:
	if 1+uidLen+1 != int(bodyLen) {
		return nil, fmt.Errorf(
			"%w: bodyLen=%d, expected=%d",
			ErrLengthMismatch, bodyLen, 1+uidLen+1,
		)
	}

	uid := string(body[1 : 1+uidLen])
	ack := body[1+uidLen]

	return &Response{
		UID: uid,
		ACK: ACK_TYPE(ack),
	}, nil
}
