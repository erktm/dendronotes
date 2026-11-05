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

// Params are the argon2id cost parameters.
type Params struct {
	Memory  uint32 // KiB
	Time    uint32
	Threads uint8
	SaltLen uint32
	KeyLen  uint32
}

// DefaultParams take roughly 100–200 ms and 64 MiB per hash on current
// server CPUs – well above the OWASP minimum for argon2id.
var DefaultParams = Params{Memory: 64 * 1024, Time: 3, Threads: 2, SaltLen: 16, KeyLen: 32}

var errInvalidHash = errors.New("not a valid argon2id hash (expected $argon2id$v=19$m=…,t=…,p=…$salt$key)")

var b64 = base64.RawStdEncoding

// Hash is a parsed argon2id password hash.
type Hash struct {
	p    Params
	salt []byte
	key  []byte
}

// HashPassword hashes password with a fresh random salt and returns the
// hash in the common PHC string format.
func HashPassword(password string, p Params) (string, error) {
	salt := make([]byte, p.SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(password), salt, p.Time, p.Memory, p.Threads, p.KeyLen)
	return encode(p, salt, key), nil
}

func encode(p Params, salt, key []byte) string {
	return fmt.Sprintf("$argon2id$v=%d$%s$%s$%s", argon2.Version, paramString(p), b64.EncodeToString(salt), b64.EncodeToString(key))
}

func paramString(p Params) string {
	return fmt.Sprintf("m=%d,t=%d,p=%d", p.Memory, p.Time, p.Threads)
}

// ParseHash parses a PHC-formatted argon2id hash. Cost parameters are
// bounded so a hostile hash cannot make every login exhaust the machine.
func ParseHash(s string) (*Hash, error) {
	parts := strings.Split(strings.TrimSpace(s), "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" || parts[2] != fmt.Sprintf("v=%d", argon2.Version) {
		return nil, errInvalidHash
	}
	var p Params
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &p.Memory, &p.Time, &p.Threads); err != nil || paramString(p) != parts[3] {
		return nil, errInvalidHash
	}
	if p.Threads < 1 || p.Threads > 16 || p.Time < 1 || p.Time > 16 || p.Memory < 8*uint32(p.Threads) || p.Memory > 1<<20 {
		return nil, errors.New("argon2id parameters out of range (m ≤ 1 GiB, t ≤ 16, p ≤ 16)")
	}
	salt, err := b64.DecodeString(parts[4])
	if err != nil || len(salt) < 8 {
		return nil, errInvalidHash
	}
	key, err := b64.DecodeString(parts[5])
	if err != nil || len(key) < 16 || len(key) > 64 {
		return nil, errInvalidHash
	}
	return &Hash{p: p, salt: salt, key: key}, nil
}

// Verify reports whether password matches, in constant time.
func (h *Hash) Verify(password string) bool {
	key := argon2.IDKey([]byte(password), h.salt, h.p.Time, h.p.Memory, h.p.Threads, uint32(len(h.key)))
	return subtle.ConstantTimeCompare(key, h.key) == 1
}
