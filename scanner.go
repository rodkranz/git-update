package main

import (
	"os"
	"path/filepath"
	"sort"
)

func discoverRepos(root string) ([]string, error) {
	var repos []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		if path != root && shouldSkipDir(d.Name()) {
			return filepath.SkipDir
		}
		gitPath := filepath.Join(path, ".git")
		info, statErr := os.Stat(gitPath)
		if statErr == nil && info.IsDir() {
			repos = append(repos, path)
			return filepath.SkipDir
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(repos)
	return repos, nil
}

func shouldSkipDir(name string) bool {
	switch name {
	case ".git", "node_modules", "vendor", ".idea", ".vscode", ".cache", "dist", "build", "target":
		return true
	default:
		return false
	}
}
