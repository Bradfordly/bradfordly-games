package panel

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAllowlistContains(t *testing.T) {
	a, err := LoadAllowlist("Bradfordly, friend@example.com, 42", "")
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		id   Identity
		want bool
	}{
		{Identity{Login: "bradfordly"}, true},
		{Identity{Email: "friend@example.com"}, true},
		{Identity{Subject: "42"}, true},
		{Identity{Login: "stranger"}, false},
		{Identity{Email: "other@example.com"}, false},
		{Identity{}, false},
	}
	for _, tc := range cases {
		if got := a.Contains(tc.id); got != tc.want {
			t.Fatalf("Contains(%+v) = %v, want %v", tc.id, got, tc.want)
		}
	}
}

func TestAllowlistEmptyDenies(t *testing.T) {
	a, err := LoadAllowlist("", "")
	if err != nil {
		t.Fatal(err)
	}
	if a.Contains(Identity{Login: "bradfordly"}) {
		t.Fatal("empty allowlist must deny")
	}
}

func TestAllowlistFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "allowlist")
	if err := os.WriteFile(path, []byte("bradfordly\n# comment\nfriend@example.com\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	a, err := LoadAllowlist("", path)
	if err != nil {
		t.Fatal(err)
	}
	if !a.Contains(Identity{Login: "bradfordly"}) {
		t.Fatal("file allowlist should contain bradfordly")
	}
	if a.Contains(Identity{Login: "stranger"}) {
		t.Fatal("file allowlist should deny stranger")
	}
}
