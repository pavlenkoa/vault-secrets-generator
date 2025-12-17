package engine

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// ChangeType represents the type of change.
type ChangeType string

// ChangeType constants define the types of changes in a diff.
const (
	ChangeAdd       ChangeType = "add"
	ChangeUpdate    ChangeType = "update"
	ChangeDelete    ChangeType = "delete" // Key will be pruned
	ChangeNone      ChangeType = "none"
	ChangeUnmanaged ChangeType = "unmanaged" // Key exists in Vault but not in config (prune=false)
)

// SecretChange represents a change to a single secret key.
type SecretChange struct {
	Key       string      `json:"key"`
	Change    ChangeType  `json:"change"`
	OldValue  string      `json:"-"` // Never expose in JSON
	NewValue  string      `json:"-"` // Never expose in JSON
	Source    ValueSource `json:"source,omitempty"`
	OldMasked string      `json:"old_value,omitempty"`
	NewMasked string      `json:"new_value,omitempty"`
}

// BlockDiff represents changes to a secret block.
type BlockDiff struct {
	Name    string         `json:"name"`
	Mount   string         `json:"mount"`
	Path    string         `json:"path"`
	Prune   bool           `json:"prune,omitempty"`
	Changes []SecretChange `json:"changes"`
}

// FullPath returns the complete Vault path as mount/path.
func (b *BlockDiff) FullPath() string {
	if b.Path == "" {
		return b.Mount
	}
	return b.Mount + "/" + b.Path
}

// Diff represents all changes across all blocks.
type Diff struct {
	Blocks []BlockDiff `json:"blocks"`
}

// HasChanges returns true if there are any changes to apply.
func (d *Diff) HasChanges() bool {
	for _, block := range d.Blocks {
		for _, change := range block.Changes {
			if change.Change == ChangeAdd || change.Change == ChangeUpdate || change.Change == ChangeDelete {
				return true
			}
		}
	}
	return false
}

// Summary returns a summary of changes.
func (d *Diff) Summary() (adds, updates, deletes, unmanaged, unchanged int) {
	for _, block := range d.Blocks {
		for _, change := range block.Changes {
			switch change.Change {
			case ChangeAdd:
				adds++
			case ChangeUpdate:
				updates++
			case ChangeDelete:
				deletes++
			case ChangeUnmanaged:
				unmanaged++
			case ChangeNone:
				unchanged++
			}
		}
	}
	return
}

// ComputeDiff computes the diff between current and desired state.
// If prune is true, unmanaged keys are marked for deletion instead of warning.
func ComputeDiff(current, desired map[string]string, sources map[string]ValueSource, prune bool) []SecretChange {
	var changes []SecretChange
	seen := make(map[string]bool)

	// Check desired keys
	for key, newValue := range desired {
		seen[key] = true
		source := sources[key]

		oldValue, exists := current[key]
		if !exists {
			changes = append(changes, SecretChange{
				Key:       key,
				Change:    ChangeAdd,
				NewValue:  newValue,
				Source:    source,
				NewMasked: maskValue(newValue),
			})
		} else if oldValue != newValue {
			changes = append(changes, SecretChange{
				Key:       key,
				Change:    ChangeUpdate,
				OldValue:  oldValue,
				NewValue:  newValue,
				Source:    source,
				OldMasked: maskValue(oldValue),
				NewMasked: maskValue(newValue),
			})
		} else {
			changes = append(changes, SecretChange{
				Key:      key,
				Change:   ChangeNone,
				OldValue: oldValue,
				NewValue: newValue,
				Source:   source,
			})
		}
	}

	// Check for keys in Vault but not in config
	for key, oldValue := range current {
		if !seen[key] {
			changeType := ChangeUnmanaged
			if prune {
				changeType = ChangeDelete
			}
			changes = append(changes, SecretChange{
				Key:       key,
				Change:    changeType,
				OldValue:  oldValue,
				OldMasked: maskValue(oldValue),
			})
		}
	}

	// Sort by key for consistent output
	sort.Slice(changes, func(i, j int) bool {
		return changes[i].Key < changes[j].Key
	})

	return changes
}

// maskValue masks a secret value for display.
func maskValue(value string) string {
	if len(value) <= 4 {
		return strings.Repeat("*", len(value))
	}
	return value[:2] + strings.Repeat("*", len(value)-4) + value[len(value)-2:]
}

// FormatDiff formats the diff for human-readable output.
func FormatDiff(diff *Diff) string {
	var sb strings.Builder

	for _, block := range diff.Blocks {
		header := fmt.Sprintf("\n=== %s (%s)", block.Name, block.FullPath())
		if block.Prune {
			header += " [prune]"
		}
		sb.WriteString(header + " ===\n")

		for _, change := range block.Changes {
			switch change.Change {
			case ChangeAdd:
				sb.WriteString(fmt.Sprintf("  + %s = %s [%s]\n", change.Key, change.NewMasked, change.Source))
			case ChangeUpdate:
				sb.WriteString(fmt.Sprintf("  ~ %s: %s -> %s [%s]\n", change.Key, change.OldMasked, change.NewMasked, change.Source))
			case ChangeDelete:
				sb.WriteString(fmt.Sprintf("  - %s = %s [pruned]\n", change.Key, change.OldMasked))
			case ChangeUnmanaged:
				sb.WriteString(fmt.Sprintf("  ? %s = %s [unmanaged]\n", change.Key, change.OldMasked))
			case ChangeNone:
				// Don't show unchanged in normal output
			}
		}
	}

	adds, updates, deletes, unmanaged, unchanged := diff.Summary()
	sb.WriteString(fmt.Sprintf("\nSummary: %d to add, %d to update, %d to delete, %d unmanaged, %d unchanged\n",
		adds, updates, deletes, unmanaged, unchanged))

	return sb.String()
}

// FormatDiffVerbose formats the diff with unchanged items shown.
func FormatDiffVerbose(diff *Diff) string {
	var sb strings.Builder

	for _, block := range diff.Blocks {
		header := fmt.Sprintf("\n=== %s (%s)", block.Name, block.FullPath())
		if block.Prune {
			header += " [prune]"
		}
		sb.WriteString(header + " ===\n")

		for _, change := range block.Changes {
			switch change.Change {
			case ChangeAdd:
				sb.WriteString(fmt.Sprintf("  + %s = %s [%s]\n", change.Key, change.NewMasked, change.Source))
			case ChangeUpdate:
				sb.WriteString(fmt.Sprintf("  ~ %s: %s -> %s [%s]\n", change.Key, change.OldMasked, change.NewMasked, change.Source))
			case ChangeDelete:
				sb.WriteString(fmt.Sprintf("  - %s = %s [pruned]\n", change.Key, change.OldMasked))
			case ChangeUnmanaged:
				sb.WriteString(fmt.Sprintf("  ? %s = %s [unmanaged]\n", change.Key, change.OldMasked))
			case ChangeNone:
				sb.WriteString(fmt.Sprintf("    %s = %s [%s]\n", change.Key, change.OldMasked, change.Source))
			}
		}
	}

	adds, updates, deletes, unmanaged, unchanged := diff.Summary()
	sb.WriteString(fmt.Sprintf("\nSummary: %d to add, %d to update, %d to delete, %d unmanaged, %d unchanged\n",
		adds, updates, deletes, unmanaged, unchanged))

	return sb.String()
}

// ToJSON converts the diff to JSON format.
func (d *Diff) ToJSON() (string, error) {
	data, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}
