package parser

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// ExtractJSON extracts a value from JSON data using jq-style dot notation.
// Examples:
//   - ".outputs.db_host.value" -> data["outputs"]["db_host"]["value"]
//   - ".items[0].name" -> data["items"][0]["name"]
func ExtractJSON(data []byte, path string) (string, error) {
	var obj interface{}
	if err := json.Unmarshal(data, &obj); err != nil {
		return "", fmt.Errorf("parsing JSON: %w", err)
	}

	return extractValue(obj, path)
}

// ExtractYAML extracts a value from YAML data using yq-style dot notation.
// Uses the same syntax as ExtractJSON.
func ExtractYAML(data []byte, path string) (string, error) {
	var obj interface{}
	if err := yaml.Unmarshal(data, &obj); err != nil {
		return "", fmt.Errorf("parsing YAML: %w", err)
	}

	// Convert map[string]interface{} to work with our extraction
	obj = normalizeYAML(obj)

	return extractValue(obj, path)
}

// normalizeYAML converts map[interface{}]interface{} to map[string]interface{}
// which is what yaml.v3 produces for maps.
func normalizeYAML(v interface{}) interface{} {
	switch vv := v.(type) {
	case map[string]interface{}:
		for k, val := range vv {
			vv[k] = normalizeYAML(val)
		}
		return vv
	case []interface{}:
		for i, val := range vv {
			vv[i] = normalizeYAML(val)
		}
		return vv
	default:
		return v
	}
}

// extractValue traverses the object using the given path.
func extractValue(obj interface{}, path string) (string, error) {
	// Remove leading dot if present
	path = strings.TrimPrefix(path, ".")

	if path == "" {
		return valueToString(obj)
	}

	parts := parsePath(path)

	current := obj
	for i, part := range parts {
		if part.isArray {
			// Array access
			arr, ok := current.([]interface{})
			if !ok {
				return "", fmt.Errorf("expected array at %s, got %T", pathUpTo(parts, i), current)
			}
			if part.index < 0 || part.index >= len(arr) {
				return "", fmt.Errorf("array index %d out of bounds (length %d) at %s", part.index, len(arr), pathUpTo(parts, i))
			}
			current = arr[part.index]
		} else {
			// Object key access
			m, ok := current.(map[string]interface{})
			if !ok {
				return "", fmt.Errorf("expected object at %s, got %T", pathUpTo(parts, i), current)
			}
			val, exists := m[part.key]
			if !exists {
				return "", fmt.Errorf("key %q not found at %s", part.key, pathUpTo(parts, i))
			}
			current = val
		}
	}

	return valueToString(current)
}

type pathPart struct {
	key     string
	isArray bool
	index   int
}

// parsePath parses a dot notation path into parts.
// Examples:
//   - "outputs.db_host.value" -> [{key: "outputs"}, {key: "db_host"}, {key: "value"}]
//   - "items[0].name" -> [{key: "items"}, {isArray: true, index: 0}, {key: "name"}]
func parsePath(path string) []pathPart {
	var parts []pathPart

	// Split by dots, but handle array notation
	segments := strings.Split(path, ".")

	for _, seg := range segments {
		if seg == "" {
			continue
		}

		// Check for array notation: key[index] or just [index]
		if idx := strings.Index(seg, "["); idx != -1 {
			// Has array notation
			if idx > 0 {
				// Key before bracket: key[index]
				parts = append(parts, pathPart{key: seg[:idx]})
			}

			// Parse array index(es) - handle multiple like [0][1]
			rest := seg[idx:]
			for len(rest) > 0 && rest[0] == '[' {
				end := strings.Index(rest, "]")
				if end == -1 {
					// Malformed, treat as key
					parts = append(parts, pathPart{key: rest})
					break
				}
				indexStr := rest[1:end]
				index, err := strconv.Atoi(indexStr)
				if err != nil {
					// Not a number, treat rest as key
					parts = append(parts, pathPart{key: rest})
					break
				}
				parts = append(parts, pathPart{isArray: true, index: index})
				rest = rest[end+1:]
			}
		} else {
			parts = append(parts, pathPart{key: seg})
		}
	}

	return parts
}

// pathUpTo returns a string representation of the path up to index i.
func pathUpTo(parts []pathPart, i int) string {
	var sb strings.Builder
	for j := 0; j <= i && j < len(parts); j++ {
		if parts[j].isArray {
			sb.WriteString(fmt.Sprintf("[%d]", parts[j].index))
		} else {
			if sb.Len() > 0 {
				sb.WriteString(".")
			}
			sb.WriteString(parts[j].key)
		}
	}
	return sb.String()
}

// valueToString converts a value to its string representation.
func valueToString(v interface{}) (string, error) {
	switch val := v.(type) {
	case string:
		return val, nil
	case float64:
		// Check if it's actually an integer
		if val == float64(int64(val)) {
			return strconv.FormatInt(int64(val), 10), nil
		}
		return strconv.FormatFloat(val, 'f', -1, 64), nil
	case int:
		return strconv.Itoa(val), nil
	case int64:
		return strconv.FormatInt(val, 10), nil
	case bool:
		return strconv.FormatBool(val), nil
	case nil:
		return "", nil
	default:
		// For complex types, return JSON representation
		b, err := json.Marshal(v)
		if err != nil {
			return "", fmt.Errorf("converting value to string: %w", err)
		}
		return string(b), nil
	}
}
