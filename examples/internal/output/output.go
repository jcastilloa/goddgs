// Package output contains shared presentation code for runnable examples.
package output

import (
	"encoding/json"
	"fmt"
	"os"
)

// Write encodes one example result as indented JSON.
func Write(value any) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

// Fail writes an example error and exits with a nonzero status.
func Fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
