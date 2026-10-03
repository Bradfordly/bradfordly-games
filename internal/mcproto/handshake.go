// Package mcproto implements the Minecraft Java handshake and status
// frames the same way itzg/mc-router's mcproto package does.
package mcproto

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"strings"
)

const (
	NextStateStatus = 1
	NextStateLogin  = 2
	maxFrameLength  = 2097151
)

type Handshake struct {
	ProtocolVersion int
	ServerAddress   string
	ServerPort      uint16
	NextState       int
}

func NormalizeHost(host string) string {
	host, _, _ = strings.Cut(host, "\x00")
	if i := strings.Index(host, "///"); i >= 0 {
		host = host[:i]
	}
	return strings.ToLower(strings.TrimSuffix(host, "."))
}

func ReadHandshake(r io.Reader) (Handshake, error) {
	payload, err := readFrame(r)
	if err != nil {
		return Handshake{}, err
	}
	packetID, err := readVarInt(payload)
	if err != nil {
		return Handshake{}, err
	}
	if packetID != 0 {
		return Handshake{}, errors.New("not a handshake packet")
	}
	proto, err := readVarInt(payload)
	if err != nil {
		return Handshake{}, err
	}
	addr, err := readString(payload)
	if err != nil {
		return Handshake{}, err
	}
	var port uint16
	if err := binary.Read(payload, binary.BigEndian, &port); err != nil {
		return Handshake{}, err
	}
	next, err := readVarInt(payload)
	if err != nil {
		return Handshake{}, err
	}
	return Handshake{
		ProtocolVersion: proto,
		ServerAddress:   NormalizeHost(addr),
		ServerPort:      port,
		NextState:       next,
	}, nil
}

func WriteHandshake(w io.Writer, hs Handshake) error {
	var body []byte
	body = appendVarInt(body, 0)
	body = appendVarInt(body, hs.ProtocolVersion)
	body = appendString(body, hs.ServerAddress)
	var port [2]byte
	binary.BigEndian.PutUint16(port[:], hs.ServerPort)
	body = append(body, port[:]...)
	body = appendVarInt(body, hs.NextState)
	return writeFrame(w, body)
}

func WriteStatusRequest(w io.Writer) error {
	return writeFrame(w, appendVarInt(nil, 0))
}

func WriteStatus(w io.Writer, motd string, protocol int) error {
	body, err := json.Marshal(map[string]any{
		"version": map[string]any{
			"name":     "Bradfordly",
			"protocol": protocol,
		},
		"players": map[string]any{
			"max":    20,
			"online": 0,
		},
		"description": map[string]any{
			"text": motd,
		},
	})
	if err != nil {
		return err
	}
	var payload []byte
	payload = appendVarInt(payload, 0)
	payload = appendString(payload, string(body))
	return writeFrame(w, payload)
}

func ReadStatusJSON(r io.Reader) (map[string]any, error) {
	payload, err := readFrame(r)
	if err != nil {
		return nil, err
	}
	packetID, err := readVarInt(payload)
	if err != nil {
		return nil, err
	}
	if packetID != 0 {
		return nil, errors.New("not a status response")
	}
	raw, err := readString(payload)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, err
	}
	return out, nil
}

func writeFrame(w io.Writer, payload []byte) error {
	if _, err := w.Write(appendVarInt(nil, len(payload))); err != nil {
		return err
	}
	_, err := w.Write(payload)
	return err
}

func readFrame(r io.Reader) (*buffer, error) {
	length, err := ReadVarInt(r)
	if err != nil {
		return nil, err
	}
	if length < 1 || length > maxFrameLength {
		return nil, errors.New("invalid frame length")
	}
	payload := make([]byte, length)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}
	return &buffer{b: payload}, nil
}

type buffer struct{ b []byte }

func (buf *buffer) Read(p []byte) (int, error) {
	if len(buf.b) == 0 {
		return 0, io.EOF
	}
	n := copy(p, buf.b)
	buf.b = buf.b[n:]
	return n, nil
}

func ReadVarInt(r io.Reader) (int, error) {
	var result, shift int
	tmp := make([]byte, 1)
	for i := 0; i < 5; i++ {
		if _, err := io.ReadFull(r, tmp); err != nil {
			return 0, err
		}
		result |= int(tmp[0]&0x7F) << shift
		if tmp[0]&0x80 == 0 {
			return result, nil
		}
		shift += 7
	}
	return 0, errors.New("varint too long")
}

func readVarInt(r io.Reader) (int, error) { return ReadVarInt(r) }

func readString(r io.Reader) (string, error) {
	n, err := ReadVarInt(r)
	if err != nil {
		return "", err
	}
	if n < 0 || n > 32767 {
		return "", errors.New("invalid string length")
	}
	b := make([]byte, n)
	if _, err := io.ReadFull(r, b); err != nil {
		return "", err
	}
	return string(b), nil
}

func appendVarInt(dst []byte, value int) []byte {
	v := uint32(value)
	for {
		b := byte(v & 0x7F)
		v >>= 7
		if v != 0 {
			b |= 0x80
		}
		dst = append(dst, b)
		if v == 0 {
			return dst
		}
	}
}

func appendString(dst []byte, s string) []byte {
	dst = appendVarInt(dst, len(s))
	return append(dst, s...)
}
