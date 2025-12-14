package vault

import (
	"context"
	"fmt"
	"strings"
)

// KVVersion represents the KV secrets engine version.
type KVVersion int

// KVVersion constants define the KV secrets engine versions.
const (
	KVVersionAuto KVVersion = 0
	KVVersion1    KVVersion = 1
	KVVersion2    KVVersion = 2
)

// KVClient handles KV secrets engine operations.
type KVClient struct {
	client  *Client
	mount   string
	version KVVersion
}

// NewKVClient creates a new KV client for the given mount path.
// If version is KVVersionAuto (0), it will be auto-detected.
func NewKVClient(client *Client, mount string, version KVVersion) (*KVClient, error) {
	// Clean up mount path
	mount = strings.Trim(mount, "/")

	kv := &KVClient{
		client:  client,
		mount:   mount,
		version: version,
	}

	// Auto-detect version if not specified
	if version == KVVersionAuto {
		detected, err := kv.detectVersion()
		if err != nil {
			return nil, fmt.Errorf("detecting KV version for %s: %w", mount, err)
		}
		kv.version = detected
	}

	return kv, nil
}

// detectVersion determines the KV engine version by checking mount info.
func (kv *KVClient) detectVersion() (KVVersion, error) {
	// Try to read mount configuration
	mounts, err := kv.client.client.Sys().ListMounts()
	if err != nil {
		// Fall back to trying v2 first, then v1
		return kv.detectVersionByProbing()
	}

	mountPath := kv.mount + "/"
	mount, ok := mounts[mountPath]
	if !ok {
		return KVVersionAuto, fmt.Errorf("mount not found: %s", kv.mount)
	}

	// Check mount options for version
	if mount.Options != nil {
		if v, ok := mount.Options["version"]; ok {
			switch v {
			case "1":
				return KVVersion1, nil
			case "2":
				return KVVersion2, nil
			}
		}
	}

	// Default to v2 for kv type
	if mount.Type == "kv" {
		return KVVersion2, nil
	}

	return KVVersion1, nil
}

// detectVersionByProbing tries to determine version by probing the API.
func (kv *KVClient) detectVersionByProbing() (KVVersion, error) {
	// Try reading from v2 metadata path
	path := fmt.Sprintf("%s/config", kv.mount)
	secret, err := kv.client.Logical().Read(path)
	if err == nil && secret != nil {
		// v2 has a config endpoint
		return KVVersion2, nil
	}

	// Assume v1
	return KVVersion1, nil
}

// Read retrieves a secret from the KV store.
func (kv *KVClient) Read(ctx context.Context, path string) (map[string]interface{}, error) {
	fullPath := kv.buildReadPath(path)

	secret, err := kv.client.Logical().Read(fullPath)
	if err != nil {
		return nil, fmt.Errorf("reading secret at %s: %w", path, err)
	}

	if secret == nil {
		return nil, nil // Secret doesn't exist
	}

	// For v2, extract data from the nested structure
	if kv.version == KVVersion2 {
		if data, ok := secret.Data["data"].(map[string]interface{}); ok {
			return data, nil
		}
		return nil, nil
	}

	return secret.Data, nil
}

// Write stores a secret in the KV store.
func (kv *KVClient) Write(ctx context.Context, path string, data map[string]interface{}) error {
	fullPath := kv.buildWritePath(path)

	var writeData map[string]interface{}
	if kv.version == KVVersion2 {
		writeData = map[string]interface{}{
			"data": data,
		}
	} else {
		writeData = data
	}

	_, err := kv.client.Logical().Write(fullPath, writeData)
	if err != nil {
		return fmt.Errorf("writing secret at %s: %w", path, err)
	}

	return nil
}

// Delete removes a secret from the KV store (soft delete for v2).
func (kv *KVClient) Delete(ctx context.Context, path string) error {
	fullPath := kv.buildDeletePath(path)

	_, err := kv.client.Logical().Delete(fullPath)
	if err != nil {
		return fmt.Errorf("deleting secret at %s: %w", path, err)
	}

	return nil
}

// Destroy permanently removes a secret and all its versions (v2) or deletes (v1).
// For KV v2, this deletes the metadata which removes all versions permanently.
func (kv *KVClient) Destroy(ctx context.Context, path string) error {
	path = strings.TrimPrefix(path, "/")

	var fullPath string
	if kv.version == KVVersion2 {
		// For v2, delete metadata to permanently remove all versions
		fullPath = fmt.Sprintf("%s/metadata/%s", kv.mount, path)
	} else {
		// For v1, regular delete is permanent
		fullPath = fmt.Sprintf("%s/%s", kv.mount, path)
	}

	_, err := kv.client.Logical().Delete(fullPath)
	if err != nil {
		return fmt.Errorf("destroying secret at %s: %w", path, err)
	}

	return nil
}

// Patch updates specific keys in a secret without overwriting others (v2 only).
func (kv *KVClient) Patch(ctx context.Context, path string, data map[string]interface{}) error {
	if kv.version != KVVersion2 {
		// For v1, we need to read-modify-write
		existing, err := kv.Read(ctx, path)
		if err != nil {
			return err
		}
		if existing == nil {
			existing = make(map[string]interface{})
		}
		for k, v := range data {
			existing[k] = v
		}
		return kv.Write(ctx, path, existing)
	}

	fullPath := kv.buildWritePath(path)
	writeData := map[string]interface{}{
		"data": data,
	}

	_, err := kv.client.Logical().JSONMergePatch(ctx, fullPath, writeData)
	if err != nil {
		return fmt.Errorf("patching secret at %s: %w", path, err)
	}

	return nil
}

// buildReadPath constructs the full path for reading.
func (kv *KVClient) buildReadPath(path string) string {
	path = strings.TrimPrefix(path, "/")
	if kv.version == KVVersion2 {
		return fmt.Sprintf("%s/data/%s", kv.mount, path)
	}
	return fmt.Sprintf("%s/%s", kv.mount, path)
}

// buildWritePath constructs the full path for writing.
func (kv *KVClient) buildWritePath(path string) string {
	// Same as read path for both versions
	return kv.buildReadPath(path)
}

// buildDeletePath constructs the full path for deleting.
func (kv *KVClient) buildDeletePath(path string) string {
	path = strings.TrimPrefix(path, "/")
	if kv.version == KVVersion2 {
		// For v2, delete from data path (soft delete)
		return fmt.Sprintf("%s/data/%s", kv.mount, path)
	}
	return fmt.Sprintf("%s/%s", kv.mount, path)
}

// Version returns the detected or configured KV version.
func (kv *KVClient) Version() KVVersion {
	return kv.version
}

// Mount returns the mount path.
func (kv *KVClient) Mount() string {
	return kv.mount
}

// DeleteKeys removes specific keys from a secret by writing a new version without them.
func (kv *KVClient) DeleteKeys(ctx context.Context, path string, keys []string) error {
	// Read current secret
	current, err := kv.Read(ctx, path)
	if err != nil {
		return fmt.Errorf("reading current secret: %w", err)
	}
	if current == nil {
		return fmt.Errorf("secret not found: %s", path)
	}

	// Check if any keys exist
	keysFound := false
	for _, key := range keys {
		if _, ok := current[key]; ok {
			keysFound = true
			break
		}
	}
	if !keysFound {
		return fmt.Errorf("none of the specified keys found in secret")
	}

	// Remove specified keys
	for _, key := range keys {
		delete(current, key)
	}

	// If no keys left, delete the entire secret
	if len(current) == 0 {
		return kv.Delete(ctx, path)
	}

	// Write back without the deleted keys
	return kv.Write(ctx, path, current)
}

// DestroyVersions destroys all version data but keeps metadata (KV v2 only).
// For KV v1, this is equivalent to Delete (all deletes are permanent).
func (kv *KVClient) DestroyVersions(ctx context.Context, path string) error {
	path = strings.TrimPrefix(path, "/")

	if kv.version == KVVersion2 {
		// For v2, use the destroy endpoint to destroy all versions
		// First, get all versions from metadata
		metadataPath := fmt.Sprintf("%s/metadata/%s", kv.mount, path)
		metadata, err := kv.client.Logical().Read(metadataPath)
		if err != nil {
			return fmt.Errorf("reading metadata: %w", err)
		}
		if metadata == nil {
			return fmt.Errorf("secret not found: %s", path)
		}

		// Get versions from metadata - this is a map of version numbers to their info
		versionsMap, ok := metadata.Data["versions"].(map[string]interface{})
		if !ok || len(versionsMap) == 0 {
			return fmt.Errorf("no versions found to destroy")
		}

		// Build list of all version numbers to destroy
		var versions []int
		for versionStr := range versionsMap {
			var v int
			if _, err := fmt.Sscanf(versionStr, "%d", &v); err == nil {
				versions = append(versions, v)
			}
		}

		if len(versions) == 0 {
			return fmt.Errorf("no valid versions found to destroy")
		}

		// Destroy all versions
		destroyPath := fmt.Sprintf("%s/destroy/%s", kv.mount, path)
		_, err = kv.client.Logical().Write(destroyPath, map[string]interface{}{
			"versions": versions,
		})
		if err != nil {
			return fmt.Errorf("destroying versions: %w", err)
		}

		return nil
	}

	// For v1, regular delete is permanent
	return kv.Delete(ctx, path)
}

// DestroyMetadata permanently removes all versions and metadata (KV v2 only).
// This is an alias for Destroy() for clarity.
func (kv *KVClient) DestroyMetadata(ctx context.Context, path string) error {
	return kv.Destroy(ctx, path)
}
