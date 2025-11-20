package agent

import (
	"crypto/md5"
	"fmt"
	"regexp"
)

var nameValidator = regexp.MustCompile(`[\s<|\\/>]`)

func normalizeName(n string) string {
	if name := nameValidator.ReplaceAllString(n, "_"); name != "" {
		return name
	}

	return fmt.Sprintf("%x", md5.Sum([]byte(n)))
}
