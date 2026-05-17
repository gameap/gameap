// Security tests for POSIX shell-safe quoting.
//
// OWASP API Security Top 10:2023 — API8:2023 Security Misconfiguration
// (OS command injection): user-controlled values are substituted into a
// shell command template run on the node. Quote must wrap the value so a
// POSIX shell treats it as a single inert literal and the value cannot break
// out of its argument to execute arbitrary commands. See security review
// finding #1.
package shellescape_test

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/gameap/gameap/pkg/shellescape"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestQuote — OWASP API8:2023 OS command injection: the exact escaped form is
// asserted so the single-quote-breakout idiom ('\”) is not silently changed.
func TestQuote(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "plain_value_is_wrapped_in_single_quotes",
			input: "server",
			want:  `'server'`,
		},
		{
			name:  "empty_value_is_an_empty_quoted_literal",
			input: "",
			want:  `''`,
		},
		{
			name:  "embedded_single_quote_is_escaped_with_the_idiom",
			input: `a'b`,
			want:  `'a'\''b'`,
		},
		{
			name:  "command_separator_injection_is_neutralized",
			input: "x; rm -rf /",
			want:  `'x; rm -rf /'`,
		},
		{
			name:  "command_substitution_is_neutralized",
			input: "$(reboot)",
			want:  `'$(reboot)'`,
		},
		{
			name:  "backtick_substitution_is_neutralized",
			input: "`id`",
			want:  "'`id`'",
		},
		{
			name:  "quote_then_payload_breakout_attempt_is_escaped",
			input: `'; rm -rf / #`,
			want:  `''\''; rm -rf / #'`,
		},
		{
			name:  "double_quote_needs_no_escaping_inside_single_quotes",
			input: `a"b`,
			want:  `'a"b'`,
		},
		{
			name:  "backslash_is_literal_inside_single_quotes",
			input: `a\b`,
			want:  `'a\b'`,
		},
		{
			name:  "newline_stays_inside_the_single_quoted_literal",
			input: "line1\nline2",
			want:  "'line1\nline2'",
		},
		{
			name:  "only_a_single_quote",
			input: `'`,
			want:  `''\'''`,
		},
		{
			name:  "consecutive_single_quotes",
			input: `''`,
			want:  `''\'''\'''`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ACT
			got := shellescape.Quote(tt.input)

			// ASSERT
			assert.Equal(t, tt.want, got, "escaped form must match the POSIX single-quote idiom")
		})
	}
}

// TestQuote_RoundTripsThroughRealShell — OWASP API8:2023 OS command
// injection: the strongest evidence that a payload cannot break out is to run
// it through a real POSIX shell and verify the shell sees exactly one
// argument equal to the original bytes (no extra command executed).
func TestQuote_RoundTripsThroughRealShell(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("POSIX sh not available")
	}

	// Each payload tries a different breakout. The safety oracle is exact
	// equality: `printf %s <quoted>` must reproduce the input byte-for-byte.
	// If any payload executed, the output would differ (extra/empty output or
	// command result), so equality alone proves no breakout occurred — a
	// substring check on the payload text would be meaningless because the
	// literal payload itself contains the injected command name.
	payloads := []string{
		"plain",
		"",
		`a'b`,
		"x; rm -rf /",
		"$(echo INJECTED)",
		"`echo INJECTED`",
		`'; echo INJECTED #`,
		"a\"b",
		`a\b`,
		"with spaces and\ttabs",
		"$HOME/../etc/passwd",
		"&& shutdown -h now",
		"| cat /etc/shadow",
		"${IFS}cat${IFS}/etc/passwd",
	}

	for _, p := range payloads {
		t.Run(strings.ReplaceAll(p, " ", "_"), func(t *testing.T) {
			// ARRANGE: printf %s of the quoted value must reproduce the exact
			// input and nothing else (no second command, no expansion).
			script := "printf %s " + shellescape.Quote(p)

			// ACT — the script is built solely from a constant printf and the
			// Quote() output, which is exactly the neutralization under test.
			out, err := exec.Command(sh, "-c", script).Output()
			require.NoError(t, err)

			// ASSERT
			assert.Equal(t, p, string(out),
				"shell must see exactly the original bytes as one literal; "+
					"any deviation means the payload escaped its argument or was expanded")
		})
	}
}
