package cmd

import (
	"os"
	"path/filepath"
	"strings"
)

// fallbackFindGlob walks root and returns files whose basename matches the glob pattern.
// Skips treeIgnore directories. Used when daemon is not running.
func fallbackFindGlob(root, pattern string) []string {
	var matches []string
	filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() && treeIgnore[d.Name()] {
			return filepath.SkipDir
		}
		if d.IsDir() {
			return nil
		}
		matched, _ := filepath.Match(pattern, d.Name())
		if matched {
			rel, err := filepath.Rel(root, path)
			if err != nil {
				rel = path
			}
			matches = append(matches, rel)
		}
		return nil
	})
	return matches
}

// fallbackLocateName walks root and returns files whose basename contains name (case-insensitive).
// Skips treeIgnore directories. Used when daemon is not running.
func fallbackLocateName(root, name string) []string {
	lower := strings.ToLower(name)
	var matches []string
	filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() && treeIgnore[d.Name()] {
			return filepath.SkipDir
		}
		if d.IsDir() {
			return nil
		}
		if strings.Contains(strings.ToLower(d.Name()), lower) {
			rel, err := filepath.Rel(root, path)
			if err != nil {
				rel = path
			}
			matches = append(matches, rel)
		}
		return nil
	})
	return matches
}
