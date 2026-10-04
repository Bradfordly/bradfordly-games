package gameprofile

import (
	"fmt"
	"strconv"
	"strings"
)

const nanoCPUsPerCore = 1_000_000_000

// NanoCPUs is the Docker NanoCPUs value for the profile (1 = 1 vCPU).
func (p Profile) NanoCPUs() (int64, error) {
	n, err := strconv.ParseFloat(strings.TrimSpace(p.CPU), 64)
	if err != nil {
		return 0, fmt.Errorf("cpu %q: %w", p.CPU, err)
	}
	return int64(n * nanoCPUsPerCore), nil
}

// MemoryBytes is the Docker memory limit for the profile (Gi suffix).
func (p Profile) MemoryBytes() (int64, error) {
	raw := strings.TrimSpace(p.Memory)
	if strings.HasSuffix(raw, "Gi") {
		n, err := strconv.ParseInt(strings.TrimSuffix(raw, "Gi"), 10, 64)
		if err != nil {
			return 0, fmt.Errorf("memory %q: %w", p.Memory, err)
		}
		return n << 30, nil
	}
	return 0, fmt.Errorf("memory %q: want <n>Gi", p.Memory)
}
