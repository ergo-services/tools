package main

import (
	"bytes"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
)

// writeGenFile always writes the generated (_gen.go) file, formatting it first.
func writeGenFile(path string, src []byte) error {
	formatted, err := goFormat(src)
	if err != nil {
		// write unformatted for debugging
		_ = os.MkdirAll(filepath.Dir(path), 0755)
		_ = os.WriteFile(path, src, 0644)
		return fmt.Errorf("formatting %s: %w", path, err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("creating directory for %s: %w", path, err)
	}
	return os.WriteFile(path, formatted, 0644)
}

// writeUserFile writes a user-owned file only if it does not already exist.
func writeUserFile(path string, src []byte) error {
	if _, err := os.Stat(path); err == nil {
		// file already exists, do not overwrite
		return nil
	}
	formatted, err := goFormat(src)
	if err != nil {
		// write unformatted for debugging
		_ = os.MkdirAll(filepath.Dir(path), 0755)
		_ = os.WriteFile(path, src, 0644)
		return fmt.Errorf("formatting %s: %w", path, err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("creating directory for %s: %w", path, err)
	}
	return os.WriteFile(path, formatted, 0644)
}

// writeUserFileRaw writes a user-owned non-Go file only if it does not exist.
func writeUserFileRaw(path string, src []byte) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("creating directory for %s: %w", path, err)
	}
	return os.WriteFile(path, src, 0644)
}

// goFormat runs go/format on Go source bytes.
func goFormat(src []byte) ([]byte, error) {
	return format.Source(bytes.TrimSpace(src))
}
