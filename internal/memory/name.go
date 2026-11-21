package memory

import (
	"fmt"
	"strings"
)

const maxNameLength = 128

// NormalizeName validates a Memory name and removes a trailing .md suffix.
func NormalizeName(name string) (string, error) {
	name = strings.TrimSpace(name)
	name = strings.TrimSuffix(name, ".md")
	if name == "" {
		return "", fmt.Errorf("memory name is empty")
	}
	if name == "." || name == ".." {
		return "", fmt.Errorf("invalid memory name %q", name)
	}
	if len(name) > maxNameLength {
		return "", fmt.Errorf("memory name is too long (max %d bytes)", maxNameLength)
	}
	if strings.ContainsAny(name, "/\\\x00\n\r") {
		return "", fmt.Errorf("invalid memory name %q", name)
	}
	return name, nil
}
