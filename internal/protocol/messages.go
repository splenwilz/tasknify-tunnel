package protocol

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
)

// MessageType identifies the kind of control message.
type MessageType string

const (
	MsgRegister    MessageType = "register"
	MsgRegisterAck MessageType = "register_ack"
	MsgRegisterErr MessageType = "register_err"
	MsgHeartbeat   MessageType = "heartbeat"
	MsgHeartbeatAck MessageType = "heartbeat_ack"
	MsgDisconnect  MessageType = "disconnect"
)

// MaxMessageSize is the maximum allowed control message size (1 MB).
const MaxMessageSize = 1 << 20

// Envelope wraps all control messages with a type discriminator.
type Envelope struct {
	Type    MessageType     `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// RegisterPayload is sent by the client to request a tunnel.
type RegisterPayload struct {
	Subdomain string `json:"subdomain"`
	AuthToken string `json:"auth_token,omitempty"`
	LocalPort int    `json:"local_port"`
}

// RegisterAckPayload confirms successful tunnel registration.
type RegisterAckPayload struct {
	URL       string `json:"url"`
	Subdomain string `json:"subdomain"`
}

// RegisterErrPayload indicates a registration failure.
type RegisterErrPayload struct {
	Code    string `json:"code"`    // subdomain_taken, auth_failed, reserved, rate_limited, invalid_subdomain
	Message string `json:"message"`
}

// HeartbeatPayload carries a timestamp for latency measurement.
type HeartbeatPayload struct {
	Timestamp int64 `json:"timestamp"`
}

// WriteMessage encodes a typed message and writes it to w with a 4-byte length prefix.
// Framing is necessary because yamux streams are byte-oriented (like TCP).
func WriteMessage(w io.Writer, msgType MessageType, payload any) error {
	var rawPayload json.RawMessage
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("marshal payload: %w", err)
		}
		rawPayload = b
	}

	env := Envelope{
		Type:    msgType,
		Payload: rawPayload,
	}

	data, err := json.Marshal(env)
	if err != nil {
		return fmt.Errorf("marshal envelope: %w", err)
	}

	// Write 4-byte big-endian length prefix
	length := uint32(len(data))
	if err := binary.Write(w, binary.BigEndian, length); err != nil {
		return fmt.Errorf("write length: %w", err)
	}

	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("write data: %w", err)
	}

	return nil
}

// ReadMessage reads a length-prefixed message from r and returns the envelope.
func ReadMessage(r io.Reader) (*Envelope, error) {
	// Read 4-byte big-endian length prefix
	var length uint32
	if err := binary.Read(r, binary.BigEndian, &length); err != nil {
		return nil, fmt.Errorf("read length: %w", err)
	}

	if length > MaxMessageSize {
		return nil, fmt.Errorf("message too large: %d bytes (max %d)", length, MaxMessageSize)
	}

	data := make([]byte, length)
	if _, err := io.ReadFull(r, data); err != nil {
		return nil, fmt.Errorf("read data: %w", err)
	}

	var env Envelope
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, fmt.Errorf("unmarshal envelope: %w", err)
	}

	return &env, nil
}

// DecodePayload unmarshals the envelope's raw payload into the target.
func DecodePayload[T any](env *Envelope) (*T, error) {
	var target T
	if env.Payload == nil {
		return &target, nil
	}
	if err := json.Unmarshal(env.Payload, &target); err != nil {
		return nil, fmt.Errorf("decode payload: %w", err)
	}
	return &target, nil
}
