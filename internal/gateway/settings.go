package gateway

import "time"

const (
	DefaultIdleTimeout = 15 * time.Minute
	MinIdleTimeout     = time.Minute
	DefaultStopTimeout = 2 * time.Minute
)

// IdleTimeout returns d, the 15m default when d is zero, or at least 1m.
func IdleTimeout(d time.Duration) time.Duration {
	if d <= 0 {
		return DefaultIdleTimeout
	}
	if d < MinIdleTimeout {
		return MinIdleTimeout
	}
	return d
}

// StopTimeout returns d, or the 2m default when d is zero.
func StopTimeout(d time.Duration) time.Duration {
	if d <= 0 {
		return DefaultStopTimeout
	}
	return d
}

// ParseIdleTimeout parses a duration string. Empty uses the default.
func ParseIdleTimeout(s string) (time.Duration, error) {
	if s == "" {
		return DefaultIdleTimeout, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, err
	}
	return IdleTimeout(d), nil
}

// ParseStopTimeout parses a duration string. Empty uses the default.
func ParseStopTimeout(s string) (time.Duration, error) {
	if s == "" {
		return DefaultStopTimeout, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, err
	}
	return StopTimeout(d), nil
}
