package agenthandle

import (
	"crypto/rand"
	"encoding/hex"
)

// Generate gives a new connection a memorable label plus a random suffix.
// The database remains the authority for workspace-local uniqueness.
func Generate() (string, error) {
	var entropy [10]byte
	if _, err := rand.Read(entropy[:]); err != nil {
		return "", err
	}
	adjectives := [...]string{"Astral", "Celestial", "Ember", "Lunar", "Mystic", "Silver", "Twilight", "Verdant"}
	creatures := [...]string{"Dragon", "Griffin", "Oracle", "Phoenix", "Raven", "Sphinx", "Unicorn", "Wyvern"}
	return adjectives[int(entropy[0])%len(adjectives)] + "-" + creatures[int(entropy[1])%len(creatures)] + "-" + hex.EncodeToString(entropy[2:]), nil
}
