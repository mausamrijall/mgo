// Package mgoargon2 is password hashing glue over x/crypto/argon2:
// argon2id with OWASP-recommended defaults, encoded in standard PHC
// string format ($argon2id$v=19$m=...,t=...,p=...$salt$hash), so hashes
// interoperate with every other PHC-aware implementation. Zero MGO
// imports — fully deletable.
package mgoargon2

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Params for argon2id. The zero value is replaced by Defaults.
type Params struct {
	Memory      uint32 // KiB
	Iterations  uint32
	Parallelism uint8
	SaltLength  uint32
	KeyLength   uint32
}

// Defaults follow the OWASP password-storage cheat sheet (2nd
// recommended argon2id configuration: m=64 MiB, t=3, p=2... trimmed to
// the widely deployed 19 MiB, t=2, p=1 profile for interactive logins).
var Defaults = Params{
	Memory:      19456, // 19 MiB
	Iterations:  2,
	Parallelism: 1,
	SaltLength:  16,
	KeyLength:   32,
}

// Hash derives an argon2id PHC string from password using Defaults.
func Hash(password string) (string, error) { return HashWith(password, Defaults) }

// HashWith derives an argon2id PHC string with explicit params.
func HashWith(password string, p Params) (string, error) {
	if p == (Params{}) {
		p = Defaults
	}
	salt := make([]byte, p.SaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(password), salt, p.Iterations, p.Memory, p.Parallelism, p.KeyLength)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, p.Memory, p.Iterations, p.Parallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	), nil
}

// Verify reports whether password matches the PHC-encoded hash, in
// constant time over the derived keys.
func Verify(password, encoded string) (bool, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false, fmt.Errorf("mgoargon2: not an argon2id PHC string")
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return false, fmt.Errorf("mgoargon2: unsupported argon2 version %q", parts[2])
	}
	var p Params
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &p.Memory, &p.Iterations, &p.Parallelism); err != nil {
		return false, fmt.Errorf("mgoargon2: bad params %q", parts[3])
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, fmt.Errorf("mgoargon2: bad salt encoding")
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false, fmt.Errorf("mgoargon2: bad hash encoding")
	}
	got := argon2.IDKey([]byte(password), salt, p.Iterations, p.Memory, p.Parallelism, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}
