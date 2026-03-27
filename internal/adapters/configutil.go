package adapters

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// ReadJSONFile reads a JSON file into a map. Returns empty map if file doesn't exist.
func ReadJSONFile(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{}, nil
		}
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	if len(data) == 0 {
		return map[string]any{}, nil
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return m, nil
}

// WriteJSONFile atomically writes a map as JSON to path, creating parent dirs.
func WriteJSONFile(path string, data map[string]any) error {
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal json: %w", err)
	}
	jsonData = append(jsonData, '\n')
	return WriteFileAtomic(path, jsonData)
}

// WriteFileAtomic writes content to path atomically (temp file + rename), creating parent dirs.
func WriteFileAtomic(path string, content []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create dir %s: %w", dir, err)
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, content, 0644); err != nil {
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("rename %s to %s: %w", tmp, path, err)
	}
	return nil
}

// RemoveFileIfExists removes a file, returning nil if it doesn't exist.
func RemoveFileIfExists(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove %s: %w", path, err)
	}
	return nil
}
