package generator

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"regexp"
	"strconv"
	"strings"

	"github.com/pavlenkoa/vault-secrets-generator/internal/config"
)

const (
	lowercaseLetters = "abcdefghijklmnopqrstuvwxyz"
	uppercaseLetters = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	digits           = "0123456789"
	defaultSymbols   = "-_$@"
)

// Generate creates a random password based on the given policy.
func Generate(policy config.PasswordPolicy) (string, error) {
	if err := validatePolicy(policy); err != nil {
		return "", err
	}

	// Build character sets
	symbols := policy.SymbolCharacters
	if symbols == "" {
		symbols = defaultSymbols
	}

	letters := lowercaseLetters
	if !policy.NoUpper {
		letters += uppercaseLetters
	}

	// Calculate how many letters we need
	letterCount := policy.Length - policy.Digits - policy.Symbols
	if letterCount < 0 {
		return "", fmt.Errorf("length %d is too small for %d digits and %d symbols",
			policy.Length, policy.Digits, policy.Symbols)
	}

	// Build the password
	var password []byte
	allowRepeat := policy.AllowRepeat == nil || *policy.AllowRepeat

	// Add required digits
	chars, err := randomChars(digits, policy.Digits, allowRepeat)
	if err != nil {
		return "", fmt.Errorf("generating digits: %w", err)
	}
	password = append(password, chars...)

	// Add required symbols
	chars, err = randomChars(symbols, policy.Symbols, allowRepeat)
	if err != nil {
		return "", fmt.Errorf("generating symbols: %w", err)
	}
	password = append(password, chars...)

	// Add letters
	chars, err = randomChars(letters, letterCount, allowRepeat)
	if err != nil {
		return "", fmt.Errorf("generating letters: %w", err)
	}
	password = append(password, chars...)

	// Shuffle the password
	if err := shuffle(password); err != nil {
		return "", fmt.Errorf("shuffling password: %w", err)
	}

	return string(password), nil
}

// validatePolicy checks if the policy is valid.
func validatePolicy(policy config.PasswordPolicy) error {
	if policy.Length < 1 {
		return fmt.Errorf("length must be at least 1")
	}
	if policy.Digits < 0 {
		return fmt.Errorf("digits cannot be negative")
	}
	if policy.Symbols < 0 {
		return fmt.Errorf("symbols cannot be negative")
	}

	minRequired := policy.Digits + policy.Symbols
	if policy.Length < minRequired {
		return fmt.Errorf("length %d is too small for %d digits and %d symbols",
			policy.Length, policy.Digits, policy.Symbols)
	}

	// Check if we have enough characters when AllowRepeat is false
	allowRepeat := policy.AllowRepeat == nil || *policy.AllowRepeat
	if !allowRepeat {
		symbols := policy.SymbolCharacters
		if symbols == "" {
			symbols = defaultSymbols
		}
		letters := lowercaseLetters
		if !policy.NoUpper {
			letters += uppercaseLetters
		}

		letterCount := policy.Length - policy.Digits - policy.Symbols
		if policy.Digits > len(digits) {
			return fmt.Errorf("cannot generate %d unique digits (only %d available)", policy.Digits, len(digits))
		}
		if policy.Symbols > len(symbols) {
			return fmt.Errorf("cannot generate %d unique symbols (only %d available)", policy.Symbols, len(symbols))
		}
		if letterCount > len(letters) {
			return fmt.Errorf("cannot generate %d unique letters (only %d available)", letterCount, len(letters))
		}
	}

	return nil
}

// randomChars generates n random characters from the given charset.
func randomChars(charset string, n int, allowRepeat bool) ([]byte, error) {
	if n == 0 {
		return nil, nil
	}
	if len(charset) == 0 {
		return nil, fmt.Errorf("empty charset")
	}

	result := make([]byte, n)
	available := []byte(charset)

	for i := 0; i < n; i++ {
		if len(available) == 0 {
			return nil, fmt.Errorf("not enough unique characters")
		}

		idx, err := randomInt(len(available))
		if err != nil {
			return nil, err
		}

		result[i] = available[idx]

		if !allowRepeat {
			// Remove used character
			available = append(available[:idx], available[idx+1:]...)
		}
	}

	return result, nil
}

// randomInt returns a cryptographically random int in [0, max).
func randomInt(max int) (int, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(int64(max)))
	if err != nil {
		return 0, err
	}
	return int(n.Int64()), nil
}

// shuffle randomly reorders the bytes using Fisher-Yates algorithm.
func shuffle(data []byte) error {
	for i := len(data) - 1; i > 0; i-- {
		j, err := randomInt(i + 1)
		if err != nil {
			return err
		}
		data[i], data[j] = data[j], data[i]
	}
	return nil
}

// parsePattern matches generate or generate(params)
var parsePattern = regexp.MustCompile(`^generate(?:\(([^)]*)\))?$`)

// ParseGenerateValue parses a generate value and returns the password policy.
// It accepts "generate" or "generate(length=64, symbols=0)" syntax.
// The defaults parameter provides default values for unspecified parameters.
func ParseGenerateValue(value string, defaults config.PasswordPolicy) (config.PasswordPolicy, error) {
	value = strings.TrimSpace(value)

	matches := parsePattern.FindStringSubmatch(value)
	if matches == nil {
		return config.PasswordPolicy{}, fmt.Errorf("invalid generate syntax: %s", value)
	}

	// Start with defaults
	policy := defaults

	// No parameters, return defaults
	if matches[1] == "" {
		return policy, nil
	}

	// Parse parameters
	params := strings.Split(matches[1], ",")
	for _, param := range params {
		param = strings.TrimSpace(param)
		if param == "" {
			continue
		}

		parts := strings.SplitN(param, "=", 2)
		if len(parts) != 2 {
			return config.PasswordPolicy{}, fmt.Errorf("invalid parameter: %s", param)
		}

		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])

		switch key {
		case "length":
			n, err := strconv.Atoi(val)
			if err != nil {
				return config.PasswordPolicy{}, fmt.Errorf("invalid length value: %s", val)
			}
			policy.Length = n
		case "digits":
			n, err := strconv.Atoi(val)
			if err != nil {
				return config.PasswordPolicy{}, fmt.Errorf("invalid digits value: %s", val)
			}
			policy.Digits = n
		case "symbols":
			n, err := strconv.Atoi(val)
			if err != nil {
				return config.PasswordPolicy{}, fmt.Errorf("invalid symbols value: %s", val)
			}
			policy.Symbols = n
		case "symbolCharacters":
			policy.SymbolCharacters = val
		case "noUpper":
			b, err := strconv.ParseBool(val)
			if err != nil {
				return config.PasswordPolicy{}, fmt.Errorf("invalid noUpper value: %s", val)
			}
			policy.NoUpper = b
		case "allowRepeat":
			b, err := strconv.ParseBool(val)
			if err != nil {
				return config.PasswordPolicy{}, fmt.Errorf("invalid allowRepeat value: %s", val)
			}
			policy.AllowRepeat = &b
		default:
			return config.PasswordPolicy{}, fmt.Errorf("unknown parameter: %s", key)
		}
	}

	return policy, nil
}

// IsGenerateValue returns true if the value is a generate directive.
func IsGenerateValue(value string) bool {
	value = strings.TrimSpace(value)
	return parsePattern.MatchString(value)
}
