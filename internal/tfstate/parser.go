package tfstate

import (
	"encoding/json"
	"fmt"
	"strings"
)

// State represents a Terraform state file structure.
type State struct {
	Version int                    `json:"version"`
	Outputs map[string]OutputValue `json:"outputs"`
	Values  *StateValues           `json:"values"`
}

// StateValues contains the root module values (Terraform 0.12+ format).
type StateValues struct {
	Outputs      map[string]OutputValue `json:"outputs"`
	RootModule   *Module                `json:"root_module"`
}

// Module represents a Terraform module in the state.
type Module struct {
	Address      string                 `json:"address"`
	Outputs      map[string]OutputValue `json:"outputs"`
	ChildModules []Module               `json:"child_modules"`
}

// OutputValue represents a Terraform output value.
type OutputValue struct {
	Value     interface{} `json:"value"`
	Type      interface{} `json:"type"`
	Sensitive bool        `json:"sensitive"`
}

// Parse parses a Terraform state JSON and returns the State struct.
func Parse(data []byte) (*State, error) {
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("parsing terraform state: %w", err)
	}
	return &state, nil
}

// GetOutput extracts an output value from the state using the given path.
// Path format: output.name or output.module.modulename.outputname
func GetOutput(state *State, path string) (string, error) {
	if !strings.HasPrefix(path, "output.") {
		return "", fmt.Errorf("invalid output path: must start with 'output.': %s", path)
	}

	// Remove "output." prefix
	path = strings.TrimPrefix(path, "output.")

	// Check if it's a module output: module.modulename.outputname
	if strings.HasPrefix(path, "module.") {
		return getModuleOutput(state, path)
	}

	// It's a root output
	return getRootOutput(state, path)
}

// getRootOutput gets an output from the root outputs.
func getRootOutput(state *State, name string) (string, error) {
	// Try root-level outputs first (older format)
	if state.Outputs != nil {
		if output, ok := state.Outputs[name]; ok {
			return formatValue(output.Value)
		}
	}

	// Try values.outputs (Terraform 0.12+ format)
	if state.Values != nil && state.Values.Outputs != nil {
		if output, ok := state.Values.Outputs[name]; ok {
			return formatValue(output.Value)
		}
	}

	// Try values.root_module.outputs
	if state.Values != nil && state.Values.RootModule != nil && state.Values.RootModule.Outputs != nil {
		if output, ok := state.Values.RootModule.Outputs[name]; ok {
			return formatValue(output.Value)
		}
	}

	return "", fmt.Errorf("output not found: %s", name)
}

// getModuleOutput gets an output from a child module.
// Path format: module.modulename.outputname or module.modulename.module.nested.outputname
func getModuleOutput(state *State, path string) (string, error) {
	// Remove "module." prefix
	path = strings.TrimPrefix(path, "module.")

	// Split into parts
	parts := strings.Split(path, ".")
	if len(parts) < 2 {
		return "", fmt.Errorf("invalid module output path: %s", path)
	}

	// Find the module and output
	var modules []Module

	// Check values.root_module.child_modules
	if state.Values != nil && state.Values.RootModule != nil {
		modules = state.Values.RootModule.ChildModules
	}

	return findModuleOutput(modules, parts, "")
}

// findModuleOutput recursively finds a module output.
// parentAddress tracks the parent module path for matching nested module addresses.
func findModuleOutput(modules []Module, parts []string, parentAddress string) (string, error) {
	if len(parts) < 2 {
		return "", fmt.Errorf("invalid module path")
	}

	moduleName := parts[0]
	var targetAddress string
	if parentAddress == "" {
		targetAddress = "module." + moduleName
	} else {
		targetAddress = parentAddress + ".module." + moduleName
	}

	for _, mod := range modules {
		if mod.Address == targetAddress {
			// Check if we need to go deeper (nested module)
			if len(parts) > 2 && parts[1] == "module" {
				return findModuleOutput(mod.ChildModules, parts[2:], targetAddress)
			}

			// Get the output from this module
			outputName := parts[len(parts)-1]
			if output, ok := mod.Outputs[outputName]; ok {
				return formatValue(output.Value)
			}

			return "", fmt.Errorf("output %q not found in module %q", outputName, moduleName)
		}
	}

	return "", fmt.Errorf("module not found: %s", moduleName)
}

// formatValue converts a Terraform output value to a string.
func formatValue(v interface{}) (string, error) {
	switch val := v.(type) {
	case string:
		return val, nil
	case float64:
		// JSON numbers are float64
		if val == float64(int64(val)) {
			return fmt.Sprintf("%d", int64(val)), nil
		}
		return fmt.Sprintf("%g", val), nil
	case bool:
		return fmt.Sprintf("%t", val), nil
	case nil:
		return "", nil
	case []interface{}:
		// For lists, join with comma
		parts := make([]string, len(val))
		for i, item := range val {
			s, err := formatValue(item)
			if err != nil {
				return "", err
			}
			parts[i] = s
		}
		return strings.Join(parts, ","), nil
	case map[string]interface{}:
		// For maps, return JSON
		b, err := json.Marshal(val)
		if err != nil {
			return "", fmt.Errorf("marshaling map value: %w", err)
		}
		return string(b), nil
	default:
		return fmt.Sprintf("%v", val), nil
	}
}

// ExtractOutput is a convenience function that parses state and extracts an output.
func ExtractOutput(data []byte, path string) (string, error) {
	state, err := Parse(data)
	if err != nil {
		return "", err
	}
	return GetOutput(state, path)
}
