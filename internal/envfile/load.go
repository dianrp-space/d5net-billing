package envfile

import (
	"os"
	"strings"
)

// Load reads KEY=VALUE lines from .env into the process environment.
// Existing environment variables are not overwritten.
func Load(paths ...string) {
	if len(paths) == 0 {
		paths = []string{".env"}
	}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			key, val, ok := strings.Cut(line, "=")
			if !ok {
				continue
			}
			key = strings.TrimSpace(key)
			val = strings.TrimSpace(val)
			if key == "" || os.Getenv(key) != "" {
				continue
			}
			_ = os.Setenv(key, val)
		}
		return
	}
}
