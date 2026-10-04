package panel

import (
	"context"
	"errors"
	"testing"
)

func TestMatchEmailAndSubject(t *testing.T) {
	members := []string{"Owner@Example.com", "12345", "# comment", ""}
	if !Match(members, "", "owner@example.com") {
		t.Fatal("email should match case-insensitively")
	}
	if !Match(members, "12345", "") {
		t.Fatal("subject should match exactly")
	}
	if Match(members, "999", "other@example.com") {
		t.Fatal("unknown identity must not match")
	}
	if Match(members, "", "") {
		t.Fatal("empty identity must not match")
	}
}

func TestParseMembersJSONAndLines(t *testing.T) {
	jsonMembers, err := ParseMembers(`["a@b.com", "42"]`)
	if err != nil {
		t.Fatal(err)
	}
	if len(jsonMembers) != 2 || jsonMembers[0] != "a@b.com" {
		t.Fatalf("json members = %#v", jsonMembers)
	}
	lines, err := ParseMembers("a@b.com\n# skip\n42, other@x.com")
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 3 {
		t.Fatalf("line members = %#v", lines)
	}
	empty, err := ParseMembers("  ")
	if err != nil || empty != nil {
		t.Fatalf("empty = %#v, err=%v", empty, err)
	}
	if _, err := ParseMembers(`{"no":"array"}`); err == nil {
		t.Fatal("malformed json must fail closed")
	}
}

func TestStoreCheckerFailClosed(t *testing.T) {
	ctx := context.Background()
	_, err := (StoreChecker{}).Allow(ctx, "1", "a@b.com")
	if !errors.Is(err, ErrAllowlistUnavailable) {
		t.Fatalf("unconfigured = %v", err)
	}

	broken := StoreChecker{Store: funcStore{err: errors.New("ssm down")}, Name: "/allow"}
	ok, err := broken.Allow(ctx, "1", "a@b.com")
	if ok || !errors.Is(err, ErrAllowlistUnavailable) {
		t.Fatalf("ssm error ok=%v err=%v", ok, err)
	}

	badJSON := StoreChecker{Store: funcStore{value: `{`}, Name: "/allow"}
	ok, err = badJSON.Allow(ctx, "1", "a@b.com")
	if ok || err == nil {
		t.Fatalf("bad json ok=%v err=%v", ok, err)
	}

	good := StoreChecker{Store: funcStore{value: "friend@example.com\n"}, Name: "/allow"}
	ok, err = good.Allow(ctx, "", "friend@example.com")
	if err != nil || !ok {
		t.Fatalf("listed member ok=%v err=%v", ok, err)
	}
}

func TestStaticChecker(t *testing.T) {
	c := StaticChecker{Members: []string{"1"}}
	ok, err := c.Allow(context.Background(), "1", "")
	if err != nil || !ok {
		t.Fatalf("static ok=%v err=%v", ok, err)
	}
}

type funcStore struct {
	value string
	err   error
}

func (f funcStore) Get(context.Context, string) (string, error) {
	return f.value, f.err
}
