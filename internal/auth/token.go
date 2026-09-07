package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
)

// Token format: aj_<lookup-id>_<secret> (secret: 32 random bytes)
func Generate() (token, lookupID string, digest []byte, err error) {
	var raw [32]byte
	if _, err = rand.Read(raw[:]); err != nil {
		return
	}
	sec := base64.RawURLEncoding.EncodeToString(raw[:])
	lookup := make([]byte, 8)
	if _, err = rand.Read(lookup); err != nil {
		return
	}
	lookupID = hex.EncodeToString(lookup)
	token = fmt.Sprintf("aj_%s_%s", lookupID, sec)
	h := sha256.Sum256([]byte(sec))
	return token, lookupID, h[:], nil
}

func Digest(secret string) []byte {
	h := sha256.Sum256([]byte(secret))
	return h[:]
}

func ParseToken(authz string) (lookupID, secret string, ok bool) {
	if !strings.HasPrefix(authz, "Bearer aj_") {
		return "", "", false
	}
	parts := strings.SplitN(strings.TrimPrefix(authz, "Bearer aj_"), "_", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func ConstantEq(a, b []byte) bool {
	return subtle.ConstantTimeCompare(a, b) == 1
}
