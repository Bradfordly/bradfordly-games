package panel

import (
	"os"
	"strings"
)

// Allowlist is a fail-closed set of GitHub logins, emails, or subject IDs.
type Allowlist struct {
	entries map[string]struct{}
}

// LoadAllowlist reads identities from a comma/newline env list and an optional
// ConfigMap-mounted file. An empty result denies every user.
func LoadAllowlist(envCSV, filePath string) (Allowlist, error) {
	a := Allowlist{entries: map[string]struct{}{}}
	a.add(envCSV)
	if filePath == "" {
		return a, nil
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		return Allowlist{}, err
	}
	a.add(string(data))
	return a, nil
}

func (a Allowlist) add(raw string) {
	if a.entries == nil {
		a.entries = map[string]struct{}{}
	}
	for _, part := range strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r' || r == ';'
	}) {
		entry := strings.ToLower(strings.TrimSpace(part))
		if entry == "" || strings.HasPrefix(entry, "#") {
			continue
		}
		a.entries[entry] = struct{}{}
	}
}

// Contains reports whether the GitHub identity is invited. Empty lists deny.
func (a Allowlist) Contains(id Identity) bool {
	if len(a.entries) == 0 {
		return false
	}
	for _, key := range id.keys() {
		if _, ok := a.entries[key]; ok {
			return true
		}
	}
	return false
}

func (id Identity) keys() []string {
	out := make([]string, 0, 3)
	for _, key := range []string{id.Subject, id.Login, id.Email} {
		key = strings.ToLower(strings.TrimSpace(key))
		if key != "" {
			out = append(out, key)
		}
	}
	return out
}
