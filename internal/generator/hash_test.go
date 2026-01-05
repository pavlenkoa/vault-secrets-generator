package generator

import (
	"strings"
	"testing"

	"github.com/pavlenkoa/vault-secrets-generator/internal/config"
)

func TestHashBcrypt(t *testing.T) {
	t.Run("default cost", func(t *testing.T) {
		cfg := config.BcryptConfig{FromKey: "password"}
		hash, err := HashBcrypt("mypassword", cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify PHC format prefix
		if !strings.HasPrefix(hash, "$2a$12$") {
			t.Errorf("expected bcrypt hash to start with $2a$12$, got: %s", hash[:10])
		}

		// Verify the hash verifies correctly
		if !VerifyBcrypt(hash, "mypassword") {
			t.Error("hash should verify against correct password")
		}

		// Verify wrong password fails
		if VerifyBcrypt(hash, "wrongpassword") {
			t.Error("hash should not verify against wrong password")
		}
	})

	t.Run("custom cost", func(t *testing.T) {
		cfg := config.BcryptConfig{FromKey: "password", Cost: 10}
		hash, err := HashBcrypt("mypassword", cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !strings.HasPrefix(hash, "$2a$10$") {
			t.Errorf("expected bcrypt hash with cost 10, got: %s", hash[:10])
		}

		if !VerifyBcrypt(hash, "mypassword") {
			t.Error("hash should verify against correct password")
		}
	})

	t.Run("invalid cost", func(t *testing.T) {
		cfg := config.BcryptConfig{FromKey: "password", Cost: 50} // Too high
		_, err := HashBcrypt("mypassword", cfg)
		if err == nil {
			t.Error("expected error for invalid cost")
		}
	})
}

func TestVerifyBcrypt(t *testing.T) {
	t.Run("invalid hash format", func(t *testing.T) {
		if VerifyBcrypt("not-a-hash", "password") {
			t.Error("should return false for invalid hash")
		}
	})

	t.Run("empty hash", func(t *testing.T) {
		if VerifyBcrypt("", "password") {
			t.Error("should return false for empty hash")
		}
	})
}

func TestHashArgon2(t *testing.T) {
	t.Run("default parameters (argon2id)", func(t *testing.T) {
		cfg := config.Argon2Config{FromKey: "password"}
		hash, err := HashArgon2("mypassword", cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify PHC format
		if !strings.HasPrefix(hash, "$argon2id$v=19$m=65536,t=3,p=4$") {
			t.Errorf("expected argon2id hash with default params, got: %s", hash)
		}

		// Verify hash has 6 parts (5 $ separators)
		parts := strings.Split(hash, "$")
		if len(parts) != 6 {
			t.Errorf("expected 6 parts in hash, got %d", len(parts))
		}

		// Verify the hash verifies correctly
		if !VerifyArgon2(hash, "mypassword") {
			t.Error("hash should verify against correct password")
		}

		// Verify wrong password fails
		if VerifyArgon2(hash, "wrongpassword") {
			t.Error("hash should not verify against wrong password")
		}
	})

	t.Run("argon2i variant", func(t *testing.T) {
		cfg := config.Argon2Config{
			FromKey: "password",
			Variant: "i",
		}
		hash, err := HashArgon2("mypassword", cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !strings.HasPrefix(hash, "$argon2i$") {
			t.Errorf("expected argon2i hash, got: %s", hash)
		}

		if !VerifyArgon2(hash, "mypassword") {
			t.Error("hash should verify against correct password")
		}
	})

	t.Run("custom parameters", func(t *testing.T) {
		cfg := config.Argon2Config{
			FromKey:     "password",
			Variant:     "id",
			Memory:      32768,
			Iterations:  2,
			Parallelism: 2,
		}
		hash, err := HashArgon2("mypassword", cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !strings.Contains(hash, "m=32768,t=2,p=2") {
			t.Errorf("expected custom params in hash, got: %s", hash)
		}

		if !VerifyArgon2(hash, "mypassword") {
			t.Error("hash should verify against correct password")
		}
	})

	t.Run("invalid variant", func(t *testing.T) {
		cfg := config.Argon2Config{
			FromKey: "password",
			Variant: "invalid",
		}
		_, err := HashArgon2("mypassword", cfg)
		if err == nil {
			t.Error("expected error for invalid variant")
		}
	})
}

func TestVerifyArgon2(t *testing.T) {
	t.Run("invalid hash format", func(t *testing.T) {
		if VerifyArgon2("not-a-hash", "password") {
			t.Error("should return false for invalid hash")
		}
	})

	t.Run("empty hash", func(t *testing.T) {
		if VerifyArgon2("", "password") {
			t.Error("should return false for empty hash")
		}
	})

	t.Run("wrong algorithm prefix", func(t *testing.T) {
		if VerifyArgon2("$scrypt$...", "password") {
			t.Error("should return false for wrong algorithm")
		}
	})
}

func TestHashPbkdf2(t *testing.T) {
	t.Run("default parameters (sha512)", func(t *testing.T) {
		cfg := config.Pbkdf2Config{FromKey: "password"}
		hash, err := HashPbkdf2("mypassword", cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify PHC format
		if !strings.HasPrefix(hash, "$pbkdf2-sha512$310000$") {
			t.Errorf("expected pbkdf2-sha512 hash with default iterations, got: %s", hash)
		}

		// Verify hash has 5 parts (4 $ separators)
		parts := strings.Split(hash, "$")
		if len(parts) != 5 {
			t.Errorf("expected 5 parts in hash, got %d", len(parts))
		}

		// Verify the hash verifies correctly
		if !VerifyPbkdf2(hash, "mypassword") {
			t.Error("hash should verify against correct password")
		}

		// Verify wrong password fails
		if VerifyPbkdf2(hash, "wrongpassword") {
			t.Error("hash should not verify against wrong password")
		}
	})

	t.Run("sha256 variant", func(t *testing.T) {
		cfg := config.Pbkdf2Config{
			FromKey: "password",
			Variant: "sha256",
		}
		hash, err := HashPbkdf2("mypassword", cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !strings.HasPrefix(hash, "$pbkdf2-sha256$") {
			t.Errorf("expected pbkdf2-sha256 hash, got: %s", hash)
		}

		if !VerifyPbkdf2(hash, "mypassword") {
			t.Error("hash should verify against correct password")
		}
	})

	t.Run("custom iterations", func(t *testing.T) {
		cfg := config.Pbkdf2Config{
			FromKey:    "password",
			Variant:    "sha512",
			Iterations: 100000,
		}
		hash, err := HashPbkdf2("mypassword", cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !strings.Contains(hash, "$100000$") {
			t.Errorf("expected 100000 iterations in hash, got: %s", hash)
		}

		if !VerifyPbkdf2(hash, "mypassword") {
			t.Error("hash should verify against correct password")
		}
	})

	t.Run("invalid variant", func(t *testing.T) {
		cfg := config.Pbkdf2Config{
			FromKey: "password",
			Variant: "sha384",
		}
		_, err := HashPbkdf2("mypassword", cfg)
		if err == nil {
			t.Error("expected error for invalid variant")
		}
	})
}

func TestVerifyPbkdf2(t *testing.T) {
	t.Run("invalid hash format", func(t *testing.T) {
		if VerifyPbkdf2("not-a-hash", "password") {
			t.Error("should return false for invalid hash")
		}
	})

	t.Run("empty hash", func(t *testing.T) {
		if VerifyPbkdf2("", "password") {
			t.Error("should return false for empty hash")
		}
	})

	t.Run("wrong algorithm prefix", func(t *testing.T) {
		if VerifyPbkdf2("$argon2id$...", "password") {
			t.Error("should return false for wrong algorithm")
		}
	})
}

func TestCrossVerification(t *testing.T) {
	password := "testpassword"

	// Generate hashes with each algorithm
	bcryptHash, _ := HashBcrypt(password, config.BcryptConfig{})
	argon2Hash, _ := HashArgon2(password, config.Argon2Config{})
	pbkdf2Hash, _ := HashPbkdf2(password, config.Pbkdf2Config{})

	t.Run("bcrypt hash doesn't verify with argon2", func(t *testing.T) {
		if VerifyArgon2(bcryptHash, password) {
			t.Error("bcrypt hash should not verify with argon2")
		}
	})

	t.Run("bcrypt hash doesn't verify with pbkdf2", func(t *testing.T) {
		if VerifyPbkdf2(bcryptHash, password) {
			t.Error("bcrypt hash should not verify with pbkdf2")
		}
	})

	t.Run("argon2 hash doesn't verify with bcrypt", func(t *testing.T) {
		if VerifyBcrypt(argon2Hash, password) {
			t.Error("argon2 hash should not verify with bcrypt")
		}
	})

	t.Run("argon2 hash doesn't verify with pbkdf2", func(t *testing.T) {
		if VerifyPbkdf2(argon2Hash, password) {
			t.Error("argon2 hash should not verify with pbkdf2")
		}
	})

	t.Run("pbkdf2 hash doesn't verify with bcrypt", func(t *testing.T) {
		if VerifyBcrypt(pbkdf2Hash, password) {
			t.Error("pbkdf2 hash should not verify with bcrypt")
		}
	})

	t.Run("pbkdf2 hash doesn't verify with argon2", func(t *testing.T) {
		if VerifyArgon2(pbkdf2Hash, password) {
			t.Error("pbkdf2 hash should not verify with argon2")
		}
	})
}

func TestHashNonDeterminism(t *testing.T) {
	password := "samepassword"

	t.Run("bcrypt produces different hashes", func(t *testing.T) {
		hash1, _ := HashBcrypt(password, config.BcryptConfig{})
		hash2, _ := HashBcrypt(password, config.BcryptConfig{})

		if hash1 == hash2 {
			t.Error("bcrypt should produce different hashes due to random salt")
		}

		// Both should verify against the same password
		if !VerifyBcrypt(hash1, password) || !VerifyBcrypt(hash2, password) {
			t.Error("both hashes should verify against the same password")
		}
	})

	t.Run("argon2 produces different hashes", func(t *testing.T) {
		hash1, _ := HashArgon2(password, config.Argon2Config{})
		hash2, _ := HashArgon2(password, config.Argon2Config{})

		if hash1 == hash2 {
			t.Error("argon2 should produce different hashes due to random salt")
		}

		// Both should verify against the same password
		if !VerifyArgon2(hash1, password) || !VerifyArgon2(hash2, password) {
			t.Error("both hashes should verify against the same password")
		}
	})

	t.Run("pbkdf2 produces different hashes", func(t *testing.T) {
		hash1, _ := HashPbkdf2(password, config.Pbkdf2Config{})
		hash2, _ := HashPbkdf2(password, config.Pbkdf2Config{})

		if hash1 == hash2 {
			t.Error("pbkdf2 should produce different hashes due to random salt")
		}

		// Both should verify against the same password
		if !VerifyPbkdf2(hash1, password) || !VerifyPbkdf2(hash2, password) {
			t.Error("both hashes should verify against the same password")
		}
	})
}

func TestParseArgon2Hash(t *testing.T) {
	t.Run("valid hash", func(t *testing.T) {
		// Generate a valid hash first
		hash, _ := HashArgon2("password", config.Argon2Config{
			Variant:     "id",
			Memory:      65536,
			Iterations:  3,
			Parallelism: 4,
		})

		variant, memory, iterations, parallelism, salt, key, err := parseArgon2Hash(hash)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if variant != "id" {
			t.Errorf("expected variant 'id', got '%s'", variant)
		}
		if memory != 65536 {
			t.Errorf("expected memory 65536, got %d", memory)
		}
		if iterations != 3 {
			t.Errorf("expected iterations 3, got %d", iterations)
		}
		if parallelism != 4 {
			t.Errorf("expected parallelism 4, got %d", parallelism)
		}
		if len(salt) != 16 {
			t.Errorf("expected salt length 16, got %d", len(salt))
		}
		if len(key) != 32 {
			t.Errorf("expected key length 32, got %d", len(key))
		}
	})

	t.Run("invalid format - too few parts", func(t *testing.T) {
		_, _, _, _, _, _, err := parseArgon2Hash("$argon2id$v=19")
		if err == nil {
			t.Error("expected error for too few parts")
		}
	})

	t.Run("invalid format - wrong algorithm", func(t *testing.T) {
		_, _, _, _, _, _, err := parseArgon2Hash("$scrypt$v=1$...$...$...")
		if err == nil {
			t.Error("expected error for wrong algorithm")
		}
	})
}

func TestParsePbkdf2Hash(t *testing.T) {
	t.Run("valid hash", func(t *testing.T) {
		// Generate a valid hash first
		hash, _ := HashPbkdf2("password", config.Pbkdf2Config{
			Variant:    "sha512",
			Iterations: 310000,
		})

		variant, iterations, salt, key, err := parsePbkdf2Hash(hash)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if variant != "sha512" {
			t.Errorf("expected variant 'sha512', got '%s'", variant)
		}
		if iterations != 310000 {
			t.Errorf("expected iterations 310000, got %d", iterations)
		}
		if len(salt) != 16 {
			t.Errorf("expected salt length 16, got %d", len(salt))
		}
		if len(key) != 64 { // sha512 produces 64-byte key
			t.Errorf("expected key length 64, got %d", len(key))
		}
	})

	t.Run("sha256 variant", func(t *testing.T) {
		hash, _ := HashPbkdf2("password", config.Pbkdf2Config{Variant: "sha256"})

		variant, _, _, key, err := parsePbkdf2Hash(hash)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if variant != "sha256" {
			t.Errorf("expected variant 'sha256', got '%s'", variant)
		}
		if len(key) != 32 { // sha256 produces 32-byte key
			t.Errorf("expected key length 32, got %d", len(key))
		}
	})

	t.Run("invalid format - too few parts", func(t *testing.T) {
		_, _, _, _, err := parsePbkdf2Hash("$pbkdf2-sha512$310000")
		if err == nil {
			t.Error("expected error for too few parts")
		}
	})

	t.Run("invalid format - wrong algorithm", func(t *testing.T) {
		_, _, _, _, err := parsePbkdf2Hash("$argon2id$310000$salt$key")
		if err == nil {
			t.Error("expected error for wrong algorithm")
		}
	})
}
