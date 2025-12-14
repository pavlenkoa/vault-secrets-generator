package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
)

// Variables holds CLI --var values and environment variable overrides.
type Variables map[string]string

// ParseHCL parses HCL configuration data with the given variables.
func ParseHCL(data []byte, filename string, vars Variables) (*Config, error) {
	file, diags := hclsyntax.ParseConfig(data, filename, hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return nil, fmt.Errorf("parsing HCL: %s", diags.Error())
	}

	// Build evaluation context with custom functions
	evalCtx := buildEvalContext(vars)

	// Parse top-level blocks
	content, diags := file.Body.Content(rootSchema)
	if diags.HasErrors() {
		return nil, fmt.Errorf("parsing config structure: %s", diags.Error())
	}

	cfg := &Config{
		Secrets: make(map[string]SecretBlock),
	}

	// Process blocks
	for _, block := range content.Blocks {
		switch block.Type {
		case "vault":
			vault, err := parseVaultBlock(block, evalCtx)
			if err != nil {
				return nil, fmt.Errorf("parsing vault block: %w", err)
			}
			cfg.Vault = *vault

		case "defaults":
			defaults, err := parseDefaultsBlock(block, evalCtx)
			if err != nil {
				return nil, fmt.Errorf("parsing defaults block: %w", err)
			}
			cfg.Defaults = *defaults

		case "secret":
			if len(block.Labels) != 1 {
				return nil, fmt.Errorf("secret block requires exactly one label (path)")
			}
			// Evaluate the path label (may contain env() calls)
			path, err := evaluateStringWithInterpolation(block.Labels[0], evalCtx)
			if err != nil {
				return nil, fmt.Errorf("evaluating secret path %q: %w", block.Labels[0], err)
			}

			secretBlock, err := parseSecretBlock(block, path, evalCtx)
			if err != nil {
				return nil, fmt.Errorf("parsing secret block %q: %w", path, err)
			}

			// Use path as the key (could also use a generated name)
			cfg.Secrets[path] = *secretBlock
		}
	}

	// Apply defaults
	applyDefaults(cfg)

	// Validate
	if err := validate(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

// rootSchema defines the top-level HCL structure
var rootSchema = &hcl.BodySchema{
	Blocks: []hcl.BlockHeaderSchema{
		{Type: "vault"},
		{Type: "defaults"},
		{Type: "secret", LabelNames: []string{"path"}},
	},
}

// buildEvalContext creates the HCL evaluation context with custom functions
func buildEvalContext(vars Variables) *hcl.EvalContext {
	return &hcl.EvalContext{
		Functions: map[string]function.Function{
			"env":      makeEnvFunction(vars),
			"generate": makeGenerateFunction(),
			"json":     makeSourceFunction("json"),
			"yaml":     makeSourceFunction("yaml"),
			"raw":      makeRawFunction(),
			"vault":    makeVaultFunction(),
			"command":  makeCommandFunction(),
		},
	}
}

// makeEnvFunction creates the env() function for variable lookup
func makeEnvFunction(vars Variables) function.Function {
	return function.New(&function.Spec{
		Params: []function.Parameter{
			{Name: "name", Type: cty.String},
		},
		Type: function.StaticReturnType(cty.String),
		Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
			name := args[0].AsString()
			// CLI vars take priority over env vars
			if val, ok := vars[name]; ok {
				return cty.StringVal(val), nil
			}
			if val := os.Getenv(name); val != "" {
				return cty.StringVal(val), nil
			}
			return cty.NullVal(cty.String), fmt.Errorf("variable %q is not set", name)
		},
	})
}

// valueMarkerType is the cty object type for value markers
var valueMarkerType = cty.Object(map[string]cty.Type{
	"_type":        cty.String,
	"_strategy":    cty.String,
	"_url":         cty.String,
	"_query":       cty.String,
	"_vault_path":  cty.String,
	"_vault_key":   cty.String,
	"_command":     cty.String,
	"_length":      cty.Number,
	"_digits":      cty.Number,
	"_symbols":     cty.Number,
	"_symbol_set":  cty.String,
	"_no_upper":    cty.Bool,
	"_allow_repeat": cty.Bool,
})

// makeGenerateFunction creates the generate() function
func makeGenerateFunction() function.Function {
	return function.New(&function.Spec{
		Params: []function.Parameter{},
		VarParam: &function.Parameter{
			Name: "options",
			Type: cty.DynamicPseudoType,
		},
		Type: function.StaticReturnType(valueMarkerType),
		Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
			result := map[string]cty.Value{
				"_type":        cty.StringVal("generate"),
				"_strategy":    cty.StringVal(""),
				"_url":         cty.StringVal(""),
				"_query":       cty.StringVal(""),
				"_vault_path":  cty.StringVal(""),
				"_vault_key":   cty.StringVal(""),
				"_command":     cty.StringVal(""),
				"_length":      cty.NumberIntVal(0),
				"_digits":      cty.NumberIntVal(-1), // -1 means use default
				"_symbols":     cty.NumberIntVal(-1),
				"_symbol_set":  cty.StringVal(""),
				"_no_upper":    cty.False,
				"_allow_repeat": cty.True,
			}

			// Parse named arguments from varargs
			for _, arg := range args {
				if arg.Type().IsObjectType() {
					for k, v := range arg.AsValueMap() {
						switch k {
						case "length":
							result["_length"] = v
						case "digits":
							result["_digits"] = v
						case "symbols":
							result["_symbols"] = v
						case "symbol_set":
							result["_symbol_set"] = v
						case "no_upper":
							result["_no_upper"] = v
						case "allow_repeat":
							result["_allow_repeat"] = v
						case "strategy":
							result["_strategy"] = v
						}
					}
				}
			}

			return cty.ObjectVal(result), nil
		},
	})
}

// makeSourceFunction creates the json() or yaml() function
func makeSourceFunction(sourceType string) function.Function {
	return function.New(&function.Spec{
		Params: []function.Parameter{
			{Name: "url", Type: cty.String},
			{Name: "query", Type: cty.String},
		},
		VarParam: &function.Parameter{
			Name: "options",
			Type: cty.DynamicPseudoType,
		},
		Type: function.StaticReturnType(valueMarkerType),
		Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
			url := args[0].AsString()
			query := args[1].AsString()
			strategy := ""

			// Parse optional strategy from varargs
			for i := 2; i < len(args); i++ {
				arg := args[i]
				if arg.Type().IsObjectType() {
					if s, ok := arg.AsValueMap()["strategy"]; ok {
						strategy = s.AsString()
					}
				}
			}

			return cty.ObjectVal(map[string]cty.Value{
				"_type":        cty.StringVal(sourceType),
				"_strategy":    cty.StringVal(strategy),
				"_url":         cty.StringVal(url),
				"_query":       cty.StringVal(query),
				"_vault_path":  cty.StringVal(""),
				"_vault_key":   cty.StringVal(""),
				"_command":     cty.StringVal(""),
				"_length":      cty.NumberIntVal(0),
				"_digits":      cty.NumberIntVal(-1),
				"_symbols":     cty.NumberIntVal(-1),
				"_symbol_set":  cty.StringVal(""),
				"_no_upper":    cty.False,
				"_allow_repeat": cty.True,
			}), nil
		},
	})
}

// makeRawFunction creates the raw() function
func makeRawFunction() function.Function {
	return function.New(&function.Spec{
		Params: []function.Parameter{
			{Name: "url", Type: cty.String},
		},
		VarParam: &function.Parameter{
			Name: "options",
			Type: cty.DynamicPseudoType,
		},
		Type: function.StaticReturnType(valueMarkerType),
		Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
			url := args[0].AsString()
			strategy := ""

			// Parse optional strategy from varargs
			for i := 1; i < len(args); i++ {
				arg := args[i]
				if arg.Type().IsObjectType() {
					if s, ok := arg.AsValueMap()["strategy"]; ok {
						strategy = s.AsString()
					}
				}
			}

			return cty.ObjectVal(map[string]cty.Value{
				"_type":        cty.StringVal("raw"),
				"_strategy":    cty.StringVal(strategy),
				"_url":         cty.StringVal(url),
				"_query":       cty.StringVal(""),
				"_vault_path":  cty.StringVal(""),
				"_vault_key":   cty.StringVal(""),
				"_command":     cty.StringVal(""),
				"_length":      cty.NumberIntVal(0),
				"_digits":      cty.NumberIntVal(-1),
				"_symbols":     cty.NumberIntVal(-1),
				"_symbol_set":  cty.StringVal(""),
				"_no_upper":    cty.False,
				"_allow_repeat": cty.True,
			}), nil
		},
	})
}

// makeVaultFunction creates the vault() function
func makeVaultFunction() function.Function {
	return function.New(&function.Spec{
		Params: []function.Parameter{
			{Name: "path", Type: cty.String},
			{Name: "key", Type: cty.String},
		},
		VarParam: &function.Parameter{
			Name: "options",
			Type: cty.DynamicPseudoType,
		},
		Type: function.StaticReturnType(valueMarkerType),
		Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
			vaultPath := args[0].AsString()
			vaultKey := args[1].AsString()
			strategy := ""

			// Parse optional strategy from varargs
			for i := 2; i < len(args); i++ {
				arg := args[i]
				if arg.Type().IsObjectType() {
					if s, ok := arg.AsValueMap()["strategy"]; ok {
						strategy = s.AsString()
					}
				}
			}

			return cty.ObjectVal(map[string]cty.Value{
				"_type":        cty.StringVal("vault"),
				"_strategy":    cty.StringVal(strategy),
				"_url":         cty.StringVal(""),
				"_query":       cty.StringVal(""),
				"_vault_path":  cty.StringVal(vaultPath),
				"_vault_key":   cty.StringVal(vaultKey),
				"_command":     cty.StringVal(""),
				"_length":      cty.NumberIntVal(0),
				"_digits":      cty.NumberIntVal(-1),
				"_symbols":     cty.NumberIntVal(-1),
				"_symbol_set":  cty.StringVal(""),
				"_no_upper":    cty.False,
				"_allow_repeat": cty.True,
			}), nil
		},
	})
}

// makeCommandFunction creates the command() function
func makeCommandFunction() function.Function {
	return function.New(&function.Spec{
		Params: []function.Parameter{
			{Name: "cmd", Type: cty.String},
		},
		VarParam: &function.Parameter{
			Name: "options",
			Type: cty.DynamicPseudoType,
		},
		Type: function.StaticReturnType(valueMarkerType),
		Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
			cmd := args[0].AsString()
			strategy := ""

			// Parse optional strategy from varargs
			for i := 1; i < len(args); i++ {
				arg := args[i]
				if arg.Type().IsObjectType() {
					if s, ok := arg.AsValueMap()["strategy"]; ok {
						strategy = s.AsString()
					}
				}
			}

			return cty.ObjectVal(map[string]cty.Value{
				"_type":        cty.StringVal("command"),
				"_strategy":    cty.StringVal(strategy),
				"_url":         cty.StringVal(""),
				"_query":       cty.StringVal(""),
				"_vault_path":  cty.StringVal(""),
				"_vault_key":   cty.StringVal(""),
				"_command":     cty.StringVal(cmd),
				"_length":      cty.NumberIntVal(0),
				"_digits":      cty.NumberIntVal(-1),
				"_symbols":     cty.NumberIntVal(-1),
				"_symbol_set":  cty.StringVal(""),
				"_no_upper":    cty.False,
				"_allow_repeat": cty.True,
			}), nil
		},
	})
}

// parseVaultBlock parses the vault configuration block
func parseVaultBlock(block *hcl.Block, evalCtx *hcl.EvalContext) (*VaultConfig, error) {
	vault := &VaultConfig{}

	content, diags := block.Body.Content(&hcl.BodySchema{
		Attributes: []hcl.AttributeSchema{
			{Name: "address"},
			{Name: "namespace"},
		},
		Blocks: []hcl.BlockHeaderSchema{
			{Type: "auth"},
		},
	})
	if diags.HasErrors() {
		return nil, fmt.Errorf("%s", diags.Error())
	}

	// Parse attributes
	if attr, exists := content.Attributes["address"]; exists {
		val, diags := attr.Expr.Value(evalCtx)
		if diags.HasErrors() {
			return nil, fmt.Errorf("evaluating address: %s", diags.Error())
		}
		vault.Address = val.AsString()
	}

	if attr, exists := content.Attributes["namespace"]; exists {
		val, diags := attr.Expr.Value(evalCtx)
		if diags.HasErrors() {
			return nil, fmt.Errorf("evaluating namespace: %s", diags.Error())
		}
		vault.Namespace = val.AsString()
	}

	// Parse auth block
	for _, authBlock := range content.Blocks {
		if authBlock.Type == "auth" {
			auth, err := parseAuthBlock(authBlock, evalCtx)
			if err != nil {
				return nil, fmt.Errorf("parsing auth block: %w", err)
			}
			vault.Auth = *auth
		}
	}

	return vault, nil
}

// parseAuthBlock parses the auth configuration block
func parseAuthBlock(block *hcl.Block, evalCtx *hcl.EvalContext) (*AuthConfig, error) {
	auth := &AuthConfig{}

	content, diags := block.Body.Content(&hcl.BodySchema{
		Attributes: []hcl.AttributeSchema{
			{Name: "method"},
			{Name: "token"},
			{Name: "role"},
			{Name: "role_id"},
			{Name: "secret_id"},
			{Name: "mount_path"},
		},
	})
	if diags.HasErrors() {
		return nil, fmt.Errorf("%s", diags.Error())
	}

	attrMap := map[string]*string{
		"method":     &auth.Method,
		"token":      &auth.Token,
		"role":       &auth.Role,
		"role_id":    &auth.RoleID,
		"secret_id":  &auth.SecretID,
		"mount_path": &auth.MountPath,
	}

	for name, ptr := range attrMap {
		if attr, exists := content.Attributes[name]; exists {
			val, diags := attr.Expr.Value(evalCtx)
			if diags.HasErrors() {
				return nil, fmt.Errorf("evaluating %s: %s", name, diags.Error())
			}
			*ptr = val.AsString()
		}
	}

	return auth, nil
}

// parseDefaultsBlock parses the defaults configuration block
func parseDefaultsBlock(block *hcl.Block, evalCtx *hcl.EvalContext) (*Defaults, error) {
	defaults := &Defaults{
		Strategy: DefaultStrategyDefaults(),
		Generate: DefaultPasswordPolicy(),
	}

	content, diags := block.Body.Content(&hcl.BodySchema{
		Blocks: []hcl.BlockHeaderSchema{
			{Type: "strategy"},
			{Type: "generate"},
		},
	})
	if diags.HasErrors() {
		return nil, fmt.Errorf("%s", diags.Error())
	}

	for _, innerBlock := range content.Blocks {
		switch innerBlock.Type {
		case "strategy":
			strategy, err := parseStrategyBlock(innerBlock, evalCtx)
			if err != nil {
				return nil, fmt.Errorf("parsing strategy block: %w", err)
			}
			defaults.Strategy = *strategy

		case "generate":
			policy, err := parseGenerateBlock(innerBlock, evalCtx)
			if err != nil {
				return nil, fmt.Errorf("parsing generate block: %w", err)
			}
			defaults.Generate = *policy
		}
	}

	return defaults, nil
}

// parseStrategyBlock parses the strategy defaults block
func parseStrategyBlock(block *hcl.Block, evalCtx *hcl.EvalContext) (*StrategyDefaults, error) {
	strategy := DefaultStrategyDefaults()

	content, diags := block.Body.Content(&hcl.BodySchema{
		Attributes: []hcl.AttributeSchema{
			{Name: "generate"},
			{Name: "json"},
			{Name: "yaml"},
			{Name: "raw"},
			{Name: "static"},
			{Name: "command"},
			{Name: "vault"},
		},
	})
	if diags.HasErrors() {
		return nil, fmt.Errorf("%s", diags.Error())
	}

	attrMap := map[string]*Strategy{
		"generate": &strategy.Generate,
		"json":     &strategy.JSON,
		"yaml":     &strategy.YAML,
		"raw":      &strategy.Raw,
		"static":   &strategy.Static,
		"command":  &strategy.Command,
		"vault":    &strategy.Vault,
	}

	for name, ptr := range attrMap {
		if attr, exists := content.Attributes[name]; exists {
			val, diags := attr.Expr.Value(evalCtx)
			if diags.HasErrors() {
				return nil, fmt.Errorf("evaluating %s: %s", name, diags.Error())
			}
			*ptr = Strategy(val.AsString())
		}
	}

	return &strategy, nil
}

// parseGenerateBlock parses the generate defaults block
func parseGenerateBlock(block *hcl.Block, evalCtx *hcl.EvalContext) (*PasswordPolicy, error) {
	policy := DefaultPasswordPolicy()

	content, diags := block.Body.Content(&hcl.BodySchema{
		Attributes: []hcl.AttributeSchema{
			{Name: "length"},
			{Name: "digits"},
			{Name: "symbols"},
			{Name: "symbol_set"},
			{Name: "no_upper"},
			{Name: "allow_repeat"},
		},
	})
	if diags.HasErrors() {
		return nil, fmt.Errorf("%s", diags.Error())
	}

	if attr, exists := content.Attributes["length"]; exists {
		val, diags := attr.Expr.Value(evalCtx)
		if diags.HasErrors() {
			return nil, fmt.Errorf("evaluating length: %s", diags.Error())
		}
		n, _ := val.AsBigFloat().Int64()
		policy.Length = int(n)
	}

	if attr, exists := content.Attributes["digits"]; exists {
		val, diags := attr.Expr.Value(evalCtx)
		if diags.HasErrors() {
			return nil, fmt.Errorf("evaluating digits: %s", diags.Error())
		}
		n, _ := val.AsBigFloat().Int64()
		policy.Digits = int(n)
	}

	if attr, exists := content.Attributes["symbols"]; exists {
		val, diags := attr.Expr.Value(evalCtx)
		if diags.HasErrors() {
			return nil, fmt.Errorf("evaluating symbols: %s", diags.Error())
		}
		n, _ := val.AsBigFloat().Int64()
		policy.Symbols = int(n)
	}

	if attr, exists := content.Attributes["symbol_set"]; exists {
		val, diags := attr.Expr.Value(evalCtx)
		if diags.HasErrors() {
			return nil, fmt.Errorf("evaluating symbol_set: %s", diags.Error())
		}
		policy.SymbolCharacters = val.AsString()
	}

	if attr, exists := content.Attributes["no_upper"]; exists {
		val, diags := attr.Expr.Value(evalCtx)
		if diags.HasErrors() {
			return nil, fmt.Errorf("evaluating no_upper: %s", diags.Error())
		}
		policy.NoUpper = val.True()
	}

	if attr, exists := content.Attributes["allow_repeat"]; exists {
		val, diags := attr.Expr.Value(evalCtx)
		if diags.HasErrors() {
			return nil, fmt.Errorf("evaluating allow_repeat: %s", diags.Error())
		}
		b := val.True()
		policy.AllowRepeat = &b
	}

	return &policy, nil
}

// parseSecretBlock parses a secret block
func parseSecretBlock(block *hcl.Block, path string, evalCtx *hcl.EvalContext) (*SecretBlock, error) {
	secret := &SecretBlock{
		Path: path,
		Data: make(map[string]Value),
	}

	// Get all attributes (we handle them dynamically)
	attrs, diags := block.Body.JustAttributes()
	if diags.HasErrors() {
		return nil, fmt.Errorf("%s", diags.Error())
	}

	for name, attr := range attrs {
		// Handle special attributes
		switch name {
		case "prune":
			val, diags := attr.Expr.Value(evalCtx)
			if diags.HasErrors() {
				return nil, fmt.Errorf("evaluating prune: %s", diags.Error())
			}
			secret.Prune = val.True()
			continue

		case "version":
			val, diags := attr.Expr.Value(evalCtx)
			if diags.HasErrors() {
				return nil, fmt.Errorf("evaluating version: %s", diags.Error())
			}
			// Only treat as KV version if it's a number, otherwise treat as data key
			if val.Type() == cty.Number {
				n, _ := val.AsBigFloat().Int64()
				secret.Version = int(n)
				continue
			}
			// Fall through to treat as a data key
		}

		// All other attributes are secret data keys
		val, diags := attr.Expr.Value(evalCtx)
		if diags.HasErrors() {
			return nil, fmt.Errorf("evaluating %s: %s", name, diags.Error())
		}

		value, err := ctyValueToValue(val)
		if err != nil {
			return nil, fmt.Errorf("converting %s: %w", name, err)
		}
		secret.Data[name] = value
	}

	return secret, nil
}

// ctyValueToValue converts a cty.Value to our Value type
func ctyValueToValue(val cty.Value) (Value, error) {
	// If it's a string, it's a static value
	if val.Type() == cty.String {
		return Value{
			Type:   ValueTypeStatic,
			Static: val.AsString(),
		}, nil
	}

	// If it's our marker object, decode it
	if val.Type().IsObjectType() {
		valMap := val.AsValueMap()

		typeStr := valMap["_type"].AsString()
		strategyStr := valMap["_strategy"].AsString()

		v := Value{
			Strategy: Strategy(strategyStr),
		}

		switch typeStr {
		case "generate":
			v.Type = ValueTypeGenerate

			// Parse password policy if any custom values set
			length, _ := valMap["_length"].AsBigFloat().Int64()
			digits, _ := valMap["_digits"].AsBigFloat().Int64()
			symbols, _ := valMap["_symbols"].AsBigFloat().Int64()
			symbolSet := valMap["_symbol_set"].AsString()
			noUpper := valMap["_no_upper"].True()
			allowRepeat := valMap["_allow_repeat"].True()

			// Only set policy if any non-default values
			if length > 0 || digits >= 0 || symbols >= 0 || symbolSet != "" || noUpper || !allowRepeat {
				policy := &PasswordPolicy{}
				if length > 0 {
					policy.Length = int(length)
				}
				if digits >= 0 {
					policy.Digits = int(digits)
				}
				if symbols >= 0 {
					policy.Symbols = int(symbols)
				}
				if symbolSet != "" {
					policy.SymbolCharacters = symbolSet
				}
				policy.NoUpper = noUpper
				policy.AllowRepeat = &allowRepeat
				v.Generate = policy
			}

		case "json":
			v.Type = ValueTypeJSON
			v.URL = valMap["_url"].AsString()
			v.Query = valMap["_query"].AsString()

		case "yaml":
			v.Type = ValueTypeYAML
			v.URL = valMap["_url"].AsString()
			v.Query = valMap["_query"].AsString()

		case "raw":
			v.Type = ValueTypeRaw
			v.URL = valMap["_url"].AsString()

		case "vault":
			v.Type = ValueTypeVault
			v.VaultPath = valMap["_vault_path"].AsString()
			v.VaultKey = valMap["_vault_key"].AsString()

		case "command":
			v.Type = ValueTypeCommand
			v.Command = valMap["_command"].AsString()

		default:
			return Value{}, fmt.Errorf("unknown value type: %s", typeStr)
		}

		return v, nil
	}

	return Value{}, fmt.Errorf("unsupported value type: %s", val.Type().FriendlyName())
}

// evaluateStringWithInterpolation evaluates a string that may contain ${...} expressions
func evaluateStringWithInterpolation(s string, evalCtx *hcl.EvalContext) (string, error) {
	// Check if string contains interpolation
	if !strings.Contains(s, "${") {
		return s, nil
	}

	// Parse as HCL template
	expr, diags := hclsyntax.ParseTemplate([]byte(s), "", hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return "", fmt.Errorf("parsing template: %s", diags.Error())
	}

	val, diags := expr.Value(evalCtx)
	if diags.HasErrors() {
		return "", fmt.Errorf("evaluating template: %s", diags.Error())
	}

	return val.AsString(), nil
}

// applyDefaults applies default values to the config
func applyDefaults(cfg *Config) {
	// Apply strategy defaults if not set
	if cfg.Defaults.Strategy == (StrategyDefaults{}) {
		cfg.Defaults.Strategy = DefaultStrategyDefaults()
	}

	// Apply password policy defaults
	defaults := DefaultPasswordPolicy()
	if cfg.Defaults.Generate.Length == 0 {
		cfg.Defaults.Generate.Length = defaults.Length
	}
	if cfg.Defaults.Generate.Digits == 0 {
		cfg.Defaults.Generate.Digits = defaults.Digits
	}
	if cfg.Defaults.Generate.Symbols == 0 {
		cfg.Defaults.Generate.Symbols = defaults.Symbols
	}
	if cfg.Defaults.Generate.SymbolCharacters == "" {
		cfg.Defaults.Generate.SymbolCharacters = defaults.SymbolCharacters
	}
	if cfg.Defaults.Generate.AllowRepeat == nil {
		cfg.Defaults.Generate.AllowRepeat = defaults.AllowRepeat
	}
}

// validate validates the configuration
func validate(cfg *Config) error {
	if len(cfg.Secrets) == 0 {
		return fmt.Errorf("no secrets defined")
	}

	// Validate default generate policy
	{
		policy := cfg.Defaults.Generate
		minRequired := policy.Digits + policy.Symbols
		if !policy.NoUpper {
			minRequired++ // At least one uppercase
		}
		if policy.Length < minRequired {
			return fmt.Errorf("defaults.generate: length %d is too small for %d digits + %d symbols",
				policy.Length, policy.Digits, policy.Symbols)
		}
	}

	for name, block := range cfg.Secrets {
		if block.Path == "" {
			return fmt.Errorf("secret %q: path is required", name)
		}

		if len(block.Data) == 0 {
			return fmt.Errorf("secret %q: no data defined", name)
		}

		if block.Version != 0 && block.Version != 1 && block.Version != 2 {
			return fmt.Errorf("secret %q: version must be 1 or 2 (or 0 for auto)", name)
		}

		// Validate generate policies
		for key, val := range block.Data {
			if val.Type == ValueTypeGenerate && val.Generate != nil {
				policy := val.Generate
				if policy.Length > 0 && policy.Length < 1 {
					return fmt.Errorf("secret %q key %q: length must be at least 1", name, key)
				}

				digits := policy.Digits
				if digits < 0 {
					digits = cfg.Defaults.Generate.Digits
				}
				symbols := policy.Symbols
				if symbols < 0 {
					symbols = cfg.Defaults.Generate.Symbols
				}
				length := policy.Length
				if length == 0 {
					length = cfg.Defaults.Generate.Length
				}

				minRequired := digits + symbols
				if !policy.NoUpper {
					minRequired++ // At least one uppercase
				}
				if length < minRequired {
					return fmt.Errorf("secret %q key %q: length %d is too small for %d digits + %d symbols",
						name, key, length, digits, symbols)
				}
			}
		}
	}

	return nil
}
