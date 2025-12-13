package generator

import (
	"strings"
	"testing"
	"unicode"

	"github.com/pavlenkoa/vault-secrets-generator/internal/config"
)

func TestGenerate_DefaultPolicy(t *testing.T) {
	policy := config.DefaultPasswordPolicy()

	password, err := Generate(policy)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(password) != policy.Length {
		t.Errorf("expected length %d, got %d", policy.Length, len(password))
	}

	digitCount := countMatches(password, unicode.IsDigit)
	if digitCount < policy.Digits {
		t.Errorf("expected at least %d digits, got %d", policy.Digits, digitCount)
	}

	symbolCount := countMatches(password, func(r rune) bool {
		return strings.ContainsRune(policy.SymbolCharacters, r)
	})
	if symbolCount < policy.Symbols {
		t.Errorf("expected at least %d symbols, got %d", policy.Symbols, symbolCount)
	}
}

func TestGenerate_NoSymbols(t *testing.T) {
	policy := config.PasswordPolicy{
		Length:  16,
		Digits:  4,
		Symbols: 0,
	}

	password, err := Generate(policy)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(password) != 16 {
		t.Errorf("expected length 16, got %d", len(password))
	}

	// Should have no symbols
	for _, r := range password {
		if strings.ContainsRune(defaultSymbols, r) {
			t.Errorf("unexpected symbol in password: %c", r)
		}
	}
}

func TestGenerate_NoUpper(t *testing.T) {
	policy := config.PasswordPolicy{
		Length:  32,
		Digits:  5,
		Symbols: 5,
		NoUpper: true,
	}

	password, err := Generate(policy)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, r := range password {
		if unicode.IsUpper(r) {
			t.Errorf("unexpected uppercase letter: %c", r)
		}
	}
}

func TestGenerate_NoRepeat(t *testing.T) {
	allowRepeat := false
	policy := config.PasswordPolicy{
		Length:      20,
		Digits:      5,
		Symbols:     3,
		AllowRepeat: &allowRepeat,
	}

	password, err := Generate(policy)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check no repeated characters
	seen := make(map[rune]bool)
	for _, r := range password {
		if seen[r] {
			t.Errorf("repeated character: %c", r)
		}
		seen[r] = true
	}
}

func TestGenerate_CustomSymbols(t *testing.T) {
	policy := config.PasswordPolicy{
		Length:           16,
		Digits:           2,
		Symbols:          4,
		SymbolCharacters: "!@#",
	}

	password, err := Generate(policy)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	symbolCount := 0
	for _, r := range password {
		if strings.ContainsRune("!@#", r) {
			symbolCount++
		}
		// Ensure no default symbols leaked in
		if strings.ContainsRune(defaultSymbols, r) && !strings.ContainsRune("!@#", r) {
			t.Errorf("unexpected default symbol: %c", r)
		}
	}

	if symbolCount < 4 {
		t.Errorf("expected at least 4 symbols from !@#, got %d", symbolCount)
	}
}

func TestGenerate_LengthTooSmall(t *testing.T) {
	policy := config.PasswordPolicy{
		Length:  5,
		Digits:  5,
		Symbols: 5,
	}

	_, err := Generate(policy)
	if err == nil {
		t.Fatal("expected error for length too small")
	}
}

func TestGenerate_NoRepeatTooManyDigits(t *testing.T) {
	allowRepeat := false
	policy := config.PasswordPolicy{
		Length:      20,
		Digits:      15, // Only 10 unique digits available
		Symbols:     0,
		AllowRepeat: &allowRepeat,
	}

	_, err := Generate(policy)
	if err == nil {
		t.Fatal("expected error for too many unique digits")
	}
}

func TestGenerate_Randomness(t *testing.T) {
	policy := config.DefaultPasswordPolicy()

	// Generate multiple passwords and ensure they're different
	passwords := make(map[string]bool)
	for i := 0; i < 100; i++ {
		password, err := Generate(policy)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		passwords[password] = true
	}

	// With 32-char passwords, collisions should be essentially impossible
	if len(passwords) < 100 {
		t.Errorf("generated %d unique passwords out of 100, expected all unique", len(passwords))
	}
}

func TestIsGenerateValue(t *testing.T) {
	tests := []struct {
		value    string
		expected bool
	}{
		{"generate", true},
		{"generate()", true},
		{"generate(length=64)", true},
		{"generate(length=64, symbols=0)", true},
		{"  generate  ", true},
		{"Generate", false},
		{"GENERATE", false},
		{"generated", false},
		{"static-value", false},
		{"s3://bucket/path#output.value", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			result := IsGenerateValue(tt.value)
			if result != tt.expected {
				t.Errorf("IsGenerateValue(%q) = %v, want %v", tt.value, result, tt.expected)
			}
		})
	}
}

func TestParseGenerateValue(t *testing.T) {
	defaults := config.DefaultPasswordPolicy()

	tests := []struct {
		name     string
		value    string
		expected config.PasswordPolicy
		wantErr  bool
	}{
		{
			name:     "simple generate",
			value:    "generate",
			expected: defaults,
		},
		{
			name:  "with length",
			value: "generate(length=64)",
			expected: config.PasswordPolicy{
				Length:           64,
				Digits:           defaults.Digits,
				Symbols:          defaults.Symbols,
				SymbolCharacters: defaults.SymbolCharacters,
				NoUpper:          defaults.NoUpper,
				AllowRepeat:      defaults.AllowRepeat,
			},
		},
		{
			name:  "with multiple params",
			value: "generate(length=48, symbols=0)",
			expected: config.PasswordPolicy{
				Length:           48,
				Digits:           defaults.Digits,
				Symbols:          0,
				SymbolCharacters: defaults.SymbolCharacters,
				NoUpper:          defaults.NoUpper,
				AllowRepeat:      defaults.AllowRepeat,
			},
		},
		{
			name:  "with noUpper",
			value: "generate(length=32, noUpper=true)",
			expected: config.PasswordPolicy{
				Length:           32,
				Digits:           defaults.Digits,
				Symbols:          defaults.Symbols,
				SymbolCharacters: defaults.SymbolCharacters,
				NoUpper:          true,
				AllowRepeat:      defaults.AllowRepeat,
			},
		},
		{
			name:  "with all params",
			value: "generate(length=24, digits=3, symbols=2, noUpper=true, allowRepeat=false)",
			expected: func() config.PasswordPolicy {
				allowRepeat := false
				return config.PasswordPolicy{
					Length:           24,
					Digits:           3,
					Symbols:          2,
					SymbolCharacters: defaults.SymbolCharacters,
					NoUpper:          true,
					AllowRepeat:      &allowRepeat,
				}
			}(),
		},
		{
			name:    "invalid syntax",
			value:   "generate[length=64]",
			wantErr: true,
		},
		{
			name:    "invalid param format",
			value:   "generate(length)",
			wantErr: true,
		},
		{
			name:    "invalid length value",
			value:   "generate(length=abc)",
			wantErr: true,
		},
		{
			name:    "unknown param",
			value:   "generate(unknown=123)",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseGenerateValue(tt.value, defaults)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result.Length != tt.expected.Length {
				t.Errorf("Length = %d, want %d", result.Length, tt.expected.Length)
			}
			if result.Digits != tt.expected.Digits {
				t.Errorf("Digits = %d, want %d", result.Digits, tt.expected.Digits)
			}
			if result.Symbols != tt.expected.Symbols {
				t.Errorf("Symbols = %d, want %d", result.Symbols, tt.expected.Symbols)
			}
			if result.NoUpper != tt.expected.NoUpper {
				t.Errorf("NoUpper = %v, want %v", result.NoUpper, tt.expected.NoUpper)
			}
		})
	}
}

// countMatches counts runes in s that match the predicate.
func countMatches(s string, pred func(rune) bool) int {
	count := 0
	for _, r := range s {
		if pred(r) {
			count++
		}
	}
	return count
}
