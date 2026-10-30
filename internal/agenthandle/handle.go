// Package agenthandle defines the shared policy for human-chosen agent handles.
// Machine profile keys and group slugs have their own existing policies.
package agenthandle

import "regexp"

const MinLength = 2
const MaxLength = 64

// Pattern is suitable for both HTML pattern attributes and an anchored Go
// regexp. Escape the dash for browsers' Unicode-sets pattern syntax.
const Pattern = `[A-Za-z0-9._\-]{2,64}`
const Help = "Use 2–64 characters: A–Z, a–z, 0–9, dot (.), underscore (_) or dash (-)."

var pattern = regexp.MustCompile("^" + Pattern + "$")

// Valid preserves case and accepts only the documented ASCII character set.
func Valid(value string) bool { return pattern.MatchString(value) }
