package engine

import (
	"testing"
)

func TestComputeDiff_AddNew(t *testing.T) {
	current := map[string]string{}
	desired := map[string]string{
		"key1": "value1",
		"key2": "value2",
	}
	sources := map[string]ValueSource{
		"key1": SourceStatic,
		"key2": SourceGenerated,
	}

	changes := ComputeDiff(current, desired, sources, false)

	if len(changes) != 2 {
		t.Fatalf("expected 2 changes, got %d", len(changes))
	}

	for _, change := range changes {
		if change.Change != ChangeAdd {
			t.Errorf("expected ChangeAdd for %s, got %s", change.Key, change.Change)
		}
	}
}

func TestComputeDiff_Update(t *testing.T) {
	current := map[string]string{
		"key1": "old-value",
	}
	desired := map[string]string{
		"key1": "new-value",
	}
	sources := map[string]ValueSource{
		"key1": SourceJSON,
	}

	changes := ComputeDiff(current, desired, sources, false)

	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}

	if changes[0].Change != ChangeUpdate {
		t.Errorf("expected ChangeUpdate, got %s", changes[0].Change)
	}
	if changes[0].OldValue != "old-value" {
		t.Errorf("expected old value 'old-value', got %s", changes[0].OldValue)
	}
	if changes[0].NewValue != "new-value" {
		t.Errorf("expected new value 'new-value', got %s", changes[0].NewValue)
	}
}

func TestComputeDiff_Unmanaged(t *testing.T) {
	current := map[string]string{
		"key1": "value1",
		"key2": "value2",
	}
	desired := map[string]string{
		"key1": "value1",
	}
	sources := map[string]ValueSource{
		"key1": SourceStatic,
	}

	changes := ComputeDiff(current, desired, sources, false)

	if len(changes) != 2 {
		t.Fatalf("expected 2 changes, got %d", len(changes))
	}

	var unmanagedCount, noneCount int
	for _, change := range changes {
		switch change.Change {
		case ChangeUnmanaged:
			unmanagedCount++
			if change.Key != "key2" {
				t.Errorf("expected key2 to be unmanaged, got %s", change.Key)
			}
		case ChangeNone:
			noneCount++
		}
	}

	if unmanagedCount != 1 {
		t.Errorf("expected 1 unmanaged, got %d", unmanagedCount)
	}
	if noneCount != 1 {
		t.Errorf("expected 1 unchanged, got %d", noneCount)
	}
}

func TestComputeDiff_Prune(t *testing.T) {
	current := map[string]string{
		"key1": "value1",
		"key2": "value2",
	}
	desired := map[string]string{
		"key1": "value1",
	}
	sources := map[string]ValueSource{
		"key1": SourceStatic,
	}

	// With prune=true, unmanaged keys become deletes
	changes := ComputeDiff(current, desired, sources, true)

	if len(changes) != 2 {
		t.Fatalf("expected 2 changes, got %d", len(changes))
	}

	var deleteCount, noneCount int
	for _, change := range changes {
		switch change.Change {
		case ChangeDelete:
			deleteCount++
			if change.Key != "key2" {
				t.Errorf("expected key2 to be deleted, got %s", change.Key)
			}
		case ChangeNone:
			noneCount++
		}
	}

	if deleteCount != 1 {
		t.Errorf("expected 1 delete, got %d", deleteCount)
	}
	if noneCount != 1 {
		t.Errorf("expected 1 unchanged, got %d", noneCount)
	}
}

func TestComputeDiff_NoChange(t *testing.T) {
	current := map[string]string{
		"key1": "value1",
	}
	desired := map[string]string{
		"key1": "value1",
	}
	sources := map[string]ValueSource{
		"key1": SourceStatic,
	}

	changes := ComputeDiff(current, desired, sources, false)

	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}

	if changes[0].Change != ChangeNone {
		t.Errorf("expected ChangeNone, got %s", changes[0].Change)
	}
}

func TestDiff_HasChanges(t *testing.T) {
	tests := []struct {
		name     string
		diff     *Diff
		expected bool
	}{
		{
			name: "has add",
			diff: &Diff{
				Blocks: []BlockDiff{
					{Changes: []SecretChange{{Change: ChangeAdd}}},
				},
			},
			expected: true,
		},
		{
			name: "has update",
			diff: &Diff{
				Blocks: []BlockDiff{
					{Changes: []SecretChange{{Change: ChangeUpdate}}},
				},
			},
			expected: true,
		},
		{
			name: "has delete",
			diff: &Diff{
				Blocks: []BlockDiff{
					{Changes: []SecretChange{{Change: ChangeDelete}}},
				},
			},
			expected: true,
		},
		{
			name: "has unmanaged",
			diff: &Diff{
				Blocks: []BlockDiff{
					{Changes: []SecretChange{{Change: ChangeUnmanaged}}},
				},
			},
			expected: false, // Unmanaged keys don't count as changes
		},
		{
			name: "no changes",
			diff: &Diff{
				Blocks: []BlockDiff{
					{Changes: []SecretChange{{Change: ChangeNone}}},
				},
			},
			expected: false,
		},
		{
			name:     "empty",
			diff:     &Diff{},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.diff.HasChanges()
			if result != tt.expected {
				t.Errorf("HasChanges() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestDiff_Summary(t *testing.T) {
	diff := &Diff{
		Blocks: []BlockDiff{
			{
				Changes: []SecretChange{
					{Change: ChangeAdd},
					{Change: ChangeAdd},
					{Change: ChangeUpdate},
					{Change: ChangeDelete},
					{Change: ChangeUnmanaged},
					{Change: ChangeNone},
					{Change: ChangeNone},
				},
			},
		},
	}

	adds, updates, deletes, unmanaged, unchanged := diff.Summary()

	if adds != 2 {
		t.Errorf("expected 2 adds, got %d", adds)
	}
	if updates != 1 {
		t.Errorf("expected 1 update, got %d", updates)
	}
	if deletes != 1 {
		t.Errorf("expected 1 delete, got %d", deletes)
	}
	if unmanaged != 1 {
		t.Errorf("expected 1 unmanaged, got %d", unmanaged)
	}
	if unchanged != 2 {
		t.Errorf("expected 2 unchanged, got %d", unchanged)
	}
}

func TestMaskValue(t *testing.T) {
	tests := []struct {
		value    string
		expected string
	}{
		{"a", "*"},
		{"ab", "**"},
		{"abc", "***"},
		{"abcd", "****"},
		{"abcde", "ab*de"},
		{"password123", "pa*******23"}, // 11 chars: 2 + 7 stars + 2
		{"secret", "se**et"},           // 6 chars: 2 + 2 stars + 2
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			result := maskValue(tt.value)
			if result != tt.expected {
				t.Errorf("maskValue(%q) = %q, want %q", tt.value, result, tt.expected)
			}
		})
	}
}

func TestFormatDiff(t *testing.T) {
	diff := &Diff{
		Blocks: []BlockDiff{
			{
				Name: "main",
				Path: "kv/prod",
				Changes: []SecretChange{
					{Key: "db/password", Change: ChangeAdd, NewMasked: "se****23", Source: SourceGenerated},
					{Key: "db/host", Change: ChangeUpdate, OldMasked: "ol****ue", NewMasked: "ne****ue", Source: SourceJSON},
					{Key: "old_key", Change: ChangeDelete, OldMasked: "de****ed"},
				},
			},
		},
	}

	output := FormatDiff(diff)

	if output == "" {
		t.Error("expected non-empty output")
	}

	// Check it contains expected elements
	if !contains(output, "main") {
		t.Error("expected output to contain block name")
	}
	if !contains(output, "kv/prod") {
		t.Error("expected output to contain path")
	}
	if !contains(output, "+ db/password") {
		t.Error("expected output to contain add marker")
	}
	if !contains(output, "~ db/host") {
		t.Error("expected output to contain update marker")
	}
	if !contains(output, "- old_key") {
		t.Error("expected output to contain delete marker")
	}
}

func TestDiff_ToJSON(t *testing.T) {
	diff := &Diff{
		Blocks: []BlockDiff{
			{
				Name: "test",
				Path: "kv/test",
				Changes: []SecretChange{
					{Key: "key1", Change: ChangeAdd, Source: SourceStatic, NewMasked: "va**e1"},
				},
			},
		},
	}

	json, err := diff.ToJSON()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if json == "" {
		t.Error("expected non-empty JSON")
	}

	// Verify it's valid JSON-ish
	if !contains(json, "\"name\":") || !contains(json, "\"test\"") {
		t.Error("expected JSON to contain block name")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
