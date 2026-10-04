package mcproto

import (
	"bytes"
	"encoding/hex"
	"strings"
	"testing"
	"unicode"
)

func TestReadHandshakeStatusFixture(t *testing.T) {
	// itzg/mc-router v1.35.0 mcproto/handshake-status.hex
	raw := "10008206096c6f63616c686f737463dd01\n0100\n"
	content, err := hex.DecodeString(strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, raw))
	if err != nil {
		t.Fatal(err)
	}

	r := bytes.NewReader(content)
	hs, err := ReadHandshake(r)
	if err != nil {
		t.Fatal(err)
	}
	if hs.ServerAddress != "localhost" {
		t.Fatalf("host = %q", hs.ServerAddress)
	}
	if hs.ServerPort != 25565 {
		t.Fatalf("port = %d", hs.ServerPort)
	}
	if hs.NextState != NextStateStatus {
		t.Fatalf("next = %d, want status", hs.NextState)
	}
	if hs.ProtocolVersion != 770 {
		t.Fatalf("protocol = %d, want 770 (1.21.5)", hs.ProtocolVersion)
	}
}

func TestNormalizeHost(t *testing.T) {
	got := NormalizeHost("Survival.Games.Bradfordly.com.\x00FML")
	if got != "survival.games.bradfordly.com" {
		t.Fatalf("NormalizeHost = %q", got)
	}
}

func TestRoundTripHandshake(t *testing.T) {
	var buf bytes.Buffer
	in := Handshake{ProtocolVersion: 767, ServerAddress: "survival.example", ServerPort: 25565, NextState: NextStateLogin}
	if err := WriteHandshake(&buf, in); err != nil {
		t.Fatal(err)
	}
	out, err := ReadHandshake(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if out != in {
		t.Fatalf("got %+v want %+v", out, in)
	}
}
