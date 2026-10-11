// Package auth implements password hashing, sessions, login rate limiting and RBAC.
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// argon2id parameters (OWASP-recommended class: 64 MiB, t=3, p=2).
const (
	argonMemKiB  = 64 * 1024
	argonTime    = 3
	argonThreads = 2
	argonKeyLen  = 32
	saltLen      = 16

	MinPasswordLen = 12
	maxPasswordLen = 256 // bound CPU/memory work on attacker-supplied input
)

var ErrWeakPassword = fmt.Errorf("password must be %d-%d characters", MinPasswordLen, maxPasswordLen)

// HashPassword returns a PHC-format argon2id hash.
func HashPassword(pw string) (string, error) {
	if len(pw) < MinPasswordLen || len(pw) > maxPasswordLen {
		return "", ErrWeakPassword
	}
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(pw), salt, argonTime, argonMemKiB, argonThreads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s", argon2.Version,
		argonMemKiB, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key)), nil
}

// VerifyPassword compares in constant time. Malformed hashes verify as false.
func VerifyPassword(pw, encoded string) bool {
	if len(pw) > maxPasswordLen {
		return false
	}
	p := strings.Split(encoded, "$")
	if len(p) != 6 || p[1] != "argon2id" {
		return false
	}
	var v, m, t, th int
	if _, err := fmt.Sscanf(p[2], "v=%d", &v); err != nil || v != argon2.Version {
		return false
	}
	if _, err := fmt.Sscanf(p[3], "m=%d,t=%d,p=%d", &m, &t, &th); err != nil {
		return false
	}
	// Reject stored parameters outside sane bounds so a corrupted row cannot exhaust memory.
	if m < 8*1024 || m > 256*1024 || t < 1 || t > 10 || th < 1 || th > 16 {
		return false
	}
	salt, err1 := base64.RawStdEncoding.DecodeString(p[4])
	want, err2 := base64.RawStdEncoding.DecodeString(p[5])
	if err := errors.Join(err1, err2); err != nil || len(want) == 0 {
		return false
	}
	got := argon2.IDKey([]byte(pw), salt, uint32(t), uint32(m), uint8(th), uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}
