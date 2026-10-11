// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

package cluster

import (
	"fmt"
	"os"
	"strings"
)

// CommandEnv is the environment for a candidate binary or a workload process.
// It is an allowlist. A name that contains a credential, or an Actions
// variable, is a failure even when the allowlist would otherwise keep it.
func CommandEnv() ([]string, error) {
	out, err := filterEnv(os.Environ())
	if err != nil {
		return nil, err
	}
	return append(out, diagnosticsEnv), nil
}

func filterEnv(environ []string) ([]string, error) {
	var out []string
	for _, kv := range environ {
		key, ok := envKey(kv)
		if !ok || !allowedKey(key) {
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

func allowedKey(key string) bool {
	switch key {
	case "PATH", "HOME", "TMPDIR", "TEMP", "TMP",
		"LANG", "LC_ALL", "LC_CTYPE", "LANGUAGE",
		"USER", "LOGNAME", "SHELL", "TZ",
		"LD_LIBRARY_PATH", "PWD",
		"CGO_ENABLED", "CC", "CXX", "PKG_CONFIG_PATH",
		"SSL_CERT_FILE", "SSL_CERT_DIR",
		"HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY",
		"http_proxy", "https_proxy", "no_proxy":
		return true
	}
	if strings.HasPrefix(key, "FGDB_") || strings.HasPrefix(key, "LC_") {
		return true
	}
	return goToolchainKey(key)
}

func goToolchainKey(key string) bool {
	return strings.HasPrefix(key, "GO") && !strings.HasPrefix(key, "GOOGLE")
}

func rejectKey(key string) error {
	upper := strings.ToUpper(key)
	if strings.HasPrefix(upper, "ACTIONS_") || strings.Contains(upper, "TOKEN") ||
		strings.Contains(upper, "SECRET") || strings.Contains(upper, "PASSWORD") {
		return fmt.Errorf("refusing environment variable %s", key)
	}
	return nil
}
