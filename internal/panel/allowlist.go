package panel

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
)

// ErrAllowlistUnavailable is returned when the allowlist cannot be read.
// Callers must fail closed.
var ErrAllowlistUnavailable = errors.New("allowlist unavailable")

// Checker decides whether a GitHub subject or email may use the panel.
type Checker interface {
	Allow(ctx context.Context, subject, email string) (bool, error)
}

// StaticChecker is an in-memory allowlist for tests and local BDD.
type StaticChecker struct {
	Members []string
}

// Allow reports whether subject or email is on the static list.
func (s StaticChecker) Allow(_ context.Context, subject, email string) (bool, error) {
	return Match(s.Members, subject, email), nil
}

// StoreChecker reads the allowlist from Parameter Store on every check.
// A read or parse failure is an error; the caller must deny access.
type StoreChecker struct {
	Store ParameterStore
	Name  string
}

// Allow fetches, parses, and matches the SSM allowlist. Fail closed.
func (c StoreChecker) Allow(ctx context.Context, subject, email string) (bool, error) {
	if c.Store == nil || strings.TrimSpace(c.Name) == "" {
		return false, ErrAllowlistUnavailable
	}
	raw, err := c.Store.Get(ctx, c.Name)
	if err != nil {
		return false, errors.Join(ErrAllowlistUnavailable, err)
	}
	members, err := ParseMembers(raw)
	if err != nil {
		return false, errors.Join(ErrAllowlistUnavailable, err)
	}
	return Match(members, subject, email), nil
}

// ParseMembers accepts a JSON string array, or newline- or comma-separated
// values. Empty input is a valid empty list. Malformed JSON fails closed.
func ParseMembers(raw string) ([]string, error) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return nil, nil
	}
	if strings.HasPrefix(text, "[") || strings.HasPrefix(text, "{") {
		var members []string
		if err := json.Unmarshal([]byte(text), &members); err != nil {
			return nil, err
		}
		return members, nil
	}
	text = strings.ReplaceAll(text, ",", "\n")
	var members []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		members = append(members, line)
	}
	return members, nil
}

// Match reports whether subject or email appears in members.
// Email comparison is case-insensitive. Subjects are exact after trim.
func Match(members []string, subject, email string) bool {
	subject = strings.TrimSpace(subject)
	email = strings.ToLower(strings.TrimSpace(email))
	if subject == "" && email == "" {
		return false
	}
	for _, member := range members {
		member = strings.TrimSpace(member)
		if member == "" || strings.HasPrefix(member, "#") {
			continue
		}
		if subject != "" && member == subject {
			return true
		}
		if email != "" && strings.ToLower(member) == email {
			return true
		}
	}
	return false
}
