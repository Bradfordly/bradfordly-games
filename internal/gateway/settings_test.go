package gateway

import (
	"testing"
	"time"
)

func TestIdleTimeoutDefaultsAndFloor(t *testing.T) {
	if got := IdleTimeout(0); got != DefaultIdleTimeout {
		t.Fatalf("zero = %s, want %s", got, DefaultIdleTimeout)
	}
	if got := IdleTimeout(30 * time.Second); got != MinIdleTimeout {
		t.Fatalf("below min = %s, want %s", got, MinIdleTimeout)
	}
	if got := IdleTimeout(20 * time.Minute); got != 20*time.Minute {
		t.Fatalf("custom = %s", got)
	}
}

func TestStopTimeoutDefault(t *testing.T) {
	if got := StopTimeout(0); got != DefaultStopTimeout {
		t.Fatalf("zero = %s, want %s", got, DefaultStopTimeout)
	}
	if got := StopTimeout(90 * time.Second); got != 90*time.Second {
		t.Fatalf("custom = %s", got)
	}
}

func TestParseTimeouts(t *testing.T) {
	idle, err := ParseIdleTimeout("")
	if err != nil || idle != DefaultIdleTimeout {
		t.Fatalf("empty idle = %s, %v", idle, err)
	}
	idle, err = ParseIdleTimeout("30s")
	if err != nil || idle != MinIdleTimeout {
		t.Fatalf("30s idle = %s, %v", idle, err)
	}
	stop, err := ParseStopTimeout("")
	if err != nil || stop != DefaultStopTimeout {
		t.Fatalf("empty stop = %s, %v", stop, err)
	}
	stop, err = ParseStopTimeout("90s")
	if err != nil || stop != 90*time.Second {
		t.Fatalf("90s stop = %s, %v", stop, err)
	}
	if _, err := ParseIdleTimeout("nope"); err == nil {
		t.Fatal("want idle parse error")
	}
	if _, err := ParseStopTimeout("nope"); err == nil {
		t.Fatal("want stop parse error")
	}
}
