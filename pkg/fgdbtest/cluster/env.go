// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

package cluster

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

//go:embed allowlist.txt
var allowlistText string

const (
	ldLibraryPath = "LD_LIBRARY_PATH"
	geosLibrary   = "libgeos.so"
	geosCLibrary  = "libgeos_c.so"
)

// CommandEnv is the environment for a candidate binary or a workload process.
// Names come from allowlist.txt. A credential name on that list is a failure.
func CommandEnv() ([]string, error) {
	allowed, err := allowedNames()
	if err != nil {
		return nil, err
	}
	out, err := filterEnv(os.Environ(), allowed)
	if err != nil {
		return nil, err
	}
	return append(out, diagnosticsEnv), nil
}

// ProcessEnv is CommandEnv plus LD_LIBRARY_PATH when binary has a release lib.
// The variable is that directory alone. The runner's value is not copied.
func ProcessEnv(binary string) ([]string, error) {
	base, err := CommandEnv()
	if err != nil {
		return nil, err
	}
	return withReleaseLib(base, binary)
}

func allowedNames() (map[string]struct{}, error) {
	names, err := parseAllowlist(allowlistText)
	if err != nil {
		return nil, err
	}
	set := make(map[string]struct{}, len(names))
	for _, name := range names {
		set[name] = struct{}{}
	}
	return set, nil
}

func parseAllowlist(text string) ([]string, error) {
	var names []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.ContainsAny(line, "*?=") {
			return nil, fmt.Errorf("allowlist entry %q is not an exact name", line)
		}
		names = append(names, line)
	}
	if len(names) == 0 {
		return nil, fmt.Errorf("allowlist is empty")
	}
	return names, nil
}

func filterEnv(environ []string, allowed map[string]struct{}) ([]string, error) {
	var out []string
	for _, kv := range environ {
		key, ok := envKey(kv)
		if !ok {
			continue
		}
		if _, keep := allowed[key]; !keep {
			continue
		}
		if err := rejectKey(key); err != nil {
			return nil, err
		}
		out = append(out, kv)
	}
	return out, nil
}

func envKey(kv string) (string, bool) {
	key, _, ok := strings.Cut(kv, "=")
	if !ok || key == "" {
		return "", false
	}
	return key, true
}

func rejectKey(key string) error {
	upper := strings.ToUpper(key)
	if strings.HasPrefix(upper, "ACTIONS_") || strings.Contains(upper, "TOKEN") ||
		strings.Contains(upper, "SECRET") || strings.Contains(upper, "PASSWORD") {
		return fmt.Errorf("refusing environment variable %s", key)
	}
	return nil
}

func withReleaseLib(base []string, binary string) ([]string, error) {
	lib, err := releaseLib(binary)
	if err != nil || lib == "" {
		return base, err
	}
	out := make([]string, 0, len(base)+1)
	for _, kv := range base {
		key, ok := envKey(kv)
		if ok && key == ldLibraryPath {
			continue
		}
		out = append(out, kv)
	}
	return append(out, ldLibraryPath+"="+lib), nil
}

// releaseLib is <binary>/../lib when that directory is real and holds libgeos.
func releaseLib(binary string) (string, error) {
	if binary == "" || !strings.Contains(binary, string(os.PathSeparator)) {
		return "", nil
	}
	dir, err := filepath.Abs(filepath.Dir(binary))
	if err != nil {
		return "", err
	}
	lib := filepath.Join(dir, "lib")
	info, err := os.Lstat(lib)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("release lib is a symlink: %s", lib)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("release lib is not a directory: %s", lib)
	}
	if !dirHasGeos(lib) {
		return "", nil
	}
	return lib, nil
}

func dirHasGeos(lib string) bool {
	entries, err := os.ReadDir(lib)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		name := entry.Name()
		if name == geosLibrary || name == geosCLibrary || strings.HasPrefix(name, geosLibrary+".") {
			return true
		}
	}
	return false
}
