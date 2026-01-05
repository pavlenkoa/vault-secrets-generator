package generator

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	stdhash "hash"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/crypto/pbkdf2"

	"github.com/pavlenkoa/vault-secrets-generator/internal/config"
)

const (
	// Default bcrypt cost
	defaultBcryptCost = 12

	// Default argon2 parameters
	defaultArgon2Variant     = "id"
	defaultArgon2Memory      = 65536 // 64MB
	defaultArgon2Iterations  = 3
	defaultArgon2Parallelism = 4
	defaultArgon2KeyLength   = 32
	defaultArgon2SaltLength  = 16

	// Default PBKDF2 parameters
	defaultPbkdf2Variant    = "sha512"
	defaultPbkdf2Iterations = 310000
	defaultPbkdf2KeyLength  = 64
	defaultPbkdf2SaltLength = 16
)

// HashBcrypt generates a bcrypt hash of the password.
// Output format: $2a$cost$salt...hash (standard bcrypt format)
func HashBcrypt(password string, cfg config.BcryptConfig) (string, error) {
	cost := cfg.Cost
	if cost == 0 {
		cost = defaultBcryptCost
	}

	if cost < bcrypt.MinCost || cost > bcrypt.MaxCost {
		return "", fmt.Errorf("bcrypt cost must be between %d and %d", bcrypt.MinCost, bcrypt.MaxCost)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), cost)
	if err != nil {
		return "", fmt.Errorf("generating bcrypt hash: %w", err)
	}

	return string(hash), nil
}

// VerifyBcrypt verifies that a password matches a bcrypt hash.
func VerifyBcrypt(hash, password string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

// HashArgon2 generates an argon2 hash of the password in PHC format.
// Output format: $argon2id$v=19$m=65536,t=3,p=4$salt$hash
func HashArgon2(password string, cfg config.Argon2Config) (string, error) {
	// Apply defaults
	variant := cfg.Variant
	if variant == "" {
		variant = defaultArgon2Variant
	}
	if variant != "id" && variant != "i" {
		return "", fmt.Errorf("argon2 variant must be 'id' or 'i', got '%s'", variant)
	}

	memory := cfg.Memory
	if memory == 0 {
		memory = defaultArgon2Memory
	}

	iterations := cfg.Iterations
	if iterations == 0 {
		iterations = defaultArgon2Iterations
	}

	parallelism := cfg.Parallelism
	if parallelism == 0 {
		parallelism = defaultArgon2Parallelism
	}

	// Generate random salt
	salt := make([]byte, defaultArgon2SaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generating salt: %w", err)
	}

	// Compute hash
	var key []byte
	if variant == "id" {
		key = argon2.IDKey([]byte(password), salt, iterations, memory, parallelism, defaultArgon2KeyLength)
	} else {
		key = argon2.Key([]byte(password), salt, iterations, memory, parallelism, defaultArgon2KeyLength)
	}

	// Encode in PHC format
	// $argon2id$v=19$m=65536,t=3,p=4$salt$hash
	b64Salt := base64.RawStdEncoding.EncodeToString(salt)
	b64Key := base64.RawStdEncoding.EncodeToString(key)

	return fmt.Sprintf("$argon2%s$v=%d$m=%d,t=%d,p=%d$%s$%s",
		variant, argon2.Version, memory, iterations, parallelism, b64Salt, b64Key), nil
}

// VerifyArgon2 verifies that a password matches an argon2 hash.
func VerifyArgon2(hash, password string) bool {
	variant, memory, iterations, parallelism, salt, key, err := parseArgon2Hash(hash)
	if err != nil {
		return false
	}

	// Recompute hash with same parameters
	// #nosec G115 -- key length from decoded hash is always small (16-64 bytes)
	keyLen := uint32(len(key))
	var computedKey []byte
	if variant == "id" {
		computedKey = argon2.IDKey([]byte(password), salt, iterations, memory, parallelism, keyLen)
	} else {
		computedKey = argon2.Key([]byte(password), salt, iterations, memory, parallelism, keyLen)
	}

	// Constant-time comparison
	return subtle.ConstantTimeCompare(key, computedKey) == 1
}

// parseArgon2Hash parses a PHC-format argon2 hash string.
// Format: $argon2id$v=19$m=65536,t=3,p=4$salt$hash
func parseArgon2Hash(hash string) (variant string, memory, iterations uint32, parallelism uint8, salt, key []byte, err error) {
	parts := strings.Split(hash, "$")
	if len(parts) != 6 {
		err = fmt.Errorf("invalid argon2 hash format: expected 6 parts, got %d", len(parts))
		return
	}

	// parts[0] is empty (leading $)
	// parts[1] is "argon2id" or "argon2i"
	if !strings.HasPrefix(parts[1], "argon2") {
		err = fmt.Errorf("invalid argon2 hash: wrong algorithm %s", parts[1])
		return
	}
	variant = strings.TrimPrefix(parts[1], "argon2")
	if variant != "id" && variant != "i" {
		err = fmt.Errorf("invalid argon2 variant: %s", variant)
		return
	}

	// parts[2] is "v=19"
	if !strings.HasPrefix(parts[2], "v=") {
		err = fmt.Errorf("invalid argon2 hash: missing version")
		return
	}

	// parts[3] is "m=65536,t=3,p=4"
	params := strings.Split(parts[3], ",")
	for _, param := range params {
		kv := strings.SplitN(param, "=", 2)
		if len(kv) != 2 {
			err = fmt.Errorf("invalid argon2 parameter: %s", param)
			return
		}
		var val uint64
		val, err = strconv.ParseUint(kv[1], 10, 32)
		if err != nil {
			err = fmt.Errorf("invalid argon2 parameter value: %s", kv[1])
			return
		}
		switch kv[0] {
		case "m":
			memory = uint32(val) // #nosec G115 -- ParseUint with bitSize 32 ensures val fits
		case "t":
			iterations = uint32(val) // #nosec G115 -- ParseUint with bitSize 32 ensures val fits
		case "p":
			if val > 255 {
				err = fmt.Errorf("invalid argon2 parallelism: %d exceeds uint8 max", val)
				return
			}
			parallelism = uint8(val)
		}
	}

	// parts[4] is base64-encoded salt
	salt, err = base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		err = fmt.Errorf("invalid argon2 salt: %w", err)
		return
	}

	// parts[5] is base64-encoded key
	key, err = base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		err = fmt.Errorf("invalid argon2 key: %w", err)
		return
	}

	return
}

// HashPbkdf2 generates a PBKDF2 hash of the password in PHC format.
// Output format: $pbkdf2-sha512$iterations$salt$hash
func HashPbkdf2(password string, cfg config.Pbkdf2Config) (string, error) {
	// Apply defaults
	variant := cfg.Variant
	if variant == "" {
		variant = defaultPbkdf2Variant
	}
	if variant != "sha256" && variant != "sha512" {
		return "", fmt.Errorf("pbkdf2 variant must be 'sha256' or 'sha512', got '%s'", variant)
	}

	iterations := cfg.Iterations
	if iterations == 0 {
		iterations = defaultPbkdf2Iterations
	}

	// Determine key length and hash function based on variant
	var hashFunc func() stdhash.Hash
	var keyLength int
	if variant == "sha256" {
		hashFunc = sha256.New
		keyLength = 32
	} else {
		hashFunc = sha512.New
		keyLength = defaultPbkdf2KeyLength
	}

	// Generate random salt
	salt := make([]byte, defaultPbkdf2SaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generating salt: %w", err)
	}

	// Compute hash
	key := pbkdf2.Key([]byte(password), salt, iterations, keyLength, hashFunc)

	// Encode in PHC format
	// $pbkdf2-sha512$310000$salt$hash
	b64Salt := base64.RawStdEncoding.EncodeToString(salt)
	b64Key := base64.RawStdEncoding.EncodeToString(key)

	return fmt.Sprintf("$pbkdf2-%s$%d$%s$%s", variant, iterations, b64Salt, b64Key), nil
}

// VerifyPbkdf2 verifies that a password matches a PBKDF2 hash.
func VerifyPbkdf2(hash, password string) bool {
	variant, iterations, salt, key, err := parsePbkdf2Hash(hash)
	if err != nil {
		return false
	}

	// Determine hash function based on variant
	var hashFunc func() stdhash.Hash
	if variant == "sha256" {
		hashFunc = sha256.New
	} else {
		hashFunc = sha512.New
	}

	// Recompute hash with same parameters
	computedKey := pbkdf2.Key([]byte(password), salt, iterations, len(key), hashFunc)

	// Constant-time comparison
	return subtle.ConstantTimeCompare(key, computedKey) == 1
}

// parsePbkdf2Hash parses a PHC-format PBKDF2 hash string.
// Format: $pbkdf2-sha512$iterations$salt$hash
func parsePbkdf2Hash(hash string) (variant string, iterations int, salt, key []byte, err error) {
	parts := strings.Split(hash, "$")
	if len(parts) != 5 {
		err = fmt.Errorf("invalid pbkdf2 hash format: expected 5 parts, got %d", len(parts))
		return
	}

	// parts[0] is empty (leading $)
	// parts[1] is "pbkdf2-sha512" or "pbkdf2-sha256"
	if !strings.HasPrefix(parts[1], "pbkdf2-") {
		err = fmt.Errorf("invalid pbkdf2 hash: wrong algorithm %s", parts[1])
		return
	}
	variant = strings.TrimPrefix(parts[1], "pbkdf2-")
	if variant != "sha256" && variant != "sha512" {
		err = fmt.Errorf("invalid pbkdf2 variant: %s", variant)
		return
	}

	// parts[2] is iterations
	iterations, err = strconv.Atoi(parts[2])
	if err != nil {
		err = fmt.Errorf("invalid pbkdf2 iterations: %w", err)
		return
	}

	// parts[3] is base64-encoded salt
	salt, err = base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil {
		err = fmt.Errorf("invalid pbkdf2 salt: %w", err)
		return
	}

	// parts[4] is base64-encoded key
	key, err = base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		err = fmt.Errorf("invalid pbkdf2 key: %w", err)
		return
	}

	return
}
