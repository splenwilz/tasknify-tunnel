package protocol

import (
	"bytes"
	"testing"
)

func TestWriteAndReadMessage(t *testing.T) {
	var buf bytes.Buffer

	payload := RegisterPayload{
		Subdomain: "myapp",
		AuthToken: "secret",
		LocalPort: 3000,
	}

	if err := WriteMessage(&buf, MsgRegister, payload); err != nil {
		t.Fatalf("WriteMessage: %v", err)
	}

	env, err := ReadMessage(&buf)
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}

	if env.Type != MsgRegister {
		t.Errorf("expected type %q, got %q", MsgRegister, env.Type)
	}

	decoded, err := DecodePayload[RegisterPayload](env)
	if err != nil {
		t.Fatalf("DecodePayload: %v", err)
	}

	if decoded.Subdomain != "myapp" {
		t.Errorf("expected subdomain %q, got %q", "myapp", decoded.Subdomain)
	}
	if decoded.AuthToken != "secret" {
		t.Errorf("expected auth_token %q, got %q", "secret", decoded.AuthToken)
	}
	if decoded.LocalPort != 3000 {
		t.Errorf("expected local_port %d, got %d", 3000, decoded.LocalPort)
	}
}

func TestWriteAndReadMultipleMessages(t *testing.T) {
	var buf bytes.Buffer

	// Write multiple messages
	WriteMessage(&buf, MsgRegister, RegisterPayload{Subdomain: "app1"})
	WriteMessage(&buf, MsgHeartbeat, HeartbeatPayload{Timestamp: 12345})
	WriteMessage(&buf, MsgDisconnect, nil)

	// Read them back in order
	env1, err := ReadMessage(&buf)
	if err != nil {
		t.Fatalf("ReadMessage 1: %v", err)
	}
	if env1.Type != MsgRegister {
		t.Errorf("msg 1: expected %q, got %q", MsgRegister, env1.Type)
	}

	env2, err := ReadMessage(&buf)
	if err != nil {
		t.Fatalf("ReadMessage 2: %v", err)
	}
	if env2.Type != MsgHeartbeat {
		t.Errorf("msg 2: expected %q, got %q", MsgHeartbeat, env2.Type)
	}

	env3, err := ReadMessage(&buf)
	if err != nil {
		t.Fatalf("ReadMessage 3: %v", err)
	}
	if env3.Type != MsgDisconnect {
		t.Errorf("msg 3: expected %q, got %q", MsgDisconnect, env3.Type)
	}
}

func TestReadMessageTooLarge(t *testing.T) {
	var buf bytes.Buffer

	// Write a length prefix that exceeds MaxMessageSize
	length := uint32(MaxMessageSize + 1)
	buf.WriteByte(byte(length >> 24))
	buf.WriteByte(byte(length >> 16))
	buf.WriteByte(byte(length >> 8))
	buf.WriteByte(byte(length))

	_, err := ReadMessage(&buf)
	if err == nil {
		t.Fatal("expected error for oversized message")
	}
}

func TestWriteMessageNilPayload(t *testing.T) {
	var buf bytes.Buffer

	if err := WriteMessage(&buf, MsgDisconnect, nil); err != nil {
		t.Fatalf("WriteMessage: %v", err)
	}

	env, err := ReadMessage(&buf)
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}

	if env.Type != MsgDisconnect {
		t.Errorf("expected type %q, got %q", MsgDisconnect, env.Type)
	}
}

func TestRegisterAckPayload(t *testing.T) {
	var buf bytes.Buffer

	payload := RegisterAckPayload{
		URL:       "https://myapp.tasknify.com",
		Subdomain: "myapp",
	}

	WriteMessage(&buf, MsgRegisterAck, payload)
	env, _ := ReadMessage(&buf)

	decoded, err := DecodePayload[RegisterAckPayload](env)
	if err != nil {
		t.Fatalf("DecodePayload: %v", err)
	}

	if decoded.URL != "https://myapp.tasknify.com" {
		t.Errorf("expected URL %q, got %q", "https://myapp.tasknify.com", decoded.URL)
	}
}

func TestRegisterErrPayload(t *testing.T) {
	var buf bytes.Buffer

	payload := RegisterErrPayload{
		Code:    "subdomain_taken",
		Message: "subdomain is already in use",
	}

	WriteMessage(&buf, MsgRegisterErr, payload)
	env, _ := ReadMessage(&buf)

	decoded, err := DecodePayload[RegisterErrPayload](env)
	if err != nil {
		t.Fatalf("DecodePayload: %v", err)
	}

	if decoded.Code != "subdomain_taken" {
		t.Errorf("expected code %q, got %q", "subdomain_taken", decoded.Code)
	}
}
