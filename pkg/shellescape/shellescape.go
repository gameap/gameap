// Package shellescape provides POSIX shell-safe quoting of untrusted strings.
package shellescape

import "strings"

// Quote wraps s in single quotes so a POSIX shell treats it as one literal
// argument. Each embedded single quote is closed, escaped with a backslash,
// then reopened. Use this for any user-controlled value substituted into a
// shell command template, otherwise the value can break out of its argument
// and execute arbitrary commands on the node.
func Quote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
