package agent

import (
	"crypto/md5"
	"fmt"
	"regexp"
)

var nameValidator = regexp.MustCompile(`[\s<|\\/>]`)

// NormalizeName normalizes a name by replacing invalid characters.
// This function is exported for use by provider implementations.
func NormalizeName(n string) string {
	if name := nameValidator.ReplaceAllString(n, "_"); name != "" {
		return name
	}

	return fmt.Sprintf("%x", md5.Sum([]byte(n)))
}
