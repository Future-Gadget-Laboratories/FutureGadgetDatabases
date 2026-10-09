// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

package main

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"runtime/debug"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// backupConfigFile is deliberately made of pointers. This lets a zero value
// such as false override a built-in default while an omitted value does not.
type backupConfigFile struct {
	Backup  backupConfigValues  `yaml:"backup"`
	Restore restoreConfigValues `yaml:"restore"`
}

type backupConfigValues struct {
	Lock             *bool  `yaml:"lock"`
	LockWait         string `yaml:"lock_wait"`
	LockLease        string `yaml:"lock_lease"`
	Threads          *int   `yaml:"threads"`
	MemoryBytes      *int64 `yaml:"memory_bytes"`
	TestingMode      *bool  `yaml:"testing_mode"`
	S3CredentialMode string `yaml:"s3_credential_mode"`
}

type restoreConfigValues struct {
	SwapRestore      *bool  `yaml:"swap_restore"`
	TestingMode      *bool  `yaml:"testing_mode"`
	S3CredentialMode string `yaml:"s3_credential_mode"`
}

func configPath(args []string) string {
	for i, arg := range args {
		if arg == "--config" && i+1 < len(args) {
			return args[i+1]
		}
		if strings.HasPrefix(arg, "--config=") {
			return strings.TrimPrefix(arg, "--config=")
		}
	}
	return ""
}

func readBackupConfig(path string) (backupConfigFile, error) {
	var cfg backupConfigFile
	if path == "" {
		return cfg, nil
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return cfg, fmt.Errorf("read config %s: %w", path, err)
	}
	if err := yaml.Unmarshal(body, &cfg); err != nil {
		return cfg, fmt.Errorf("parse config %s: %w", path, err)
	}
	return cfg, nil
}

func configuredDuration(value, name string, fallback time.Duration) (time.Duration, error) {
	if value == "" {
		return fallback, nil
	}
	d, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", name, err)
	}
	if d < 0 {
		return 0, fmt.Errorf("%s must not be negative", name)
	}
	return d, nil
}

func configBool(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}
	return *value
}

func configInt(value *int, fallback int) int {
	if value == nil {
		return fallback
	}
	return *value
}

func configInt64(value *int64, fallback int64) int64 {
	if value == nil {
		return fallback
	}
	return *value
}

func validateConfig(cfg backupConfigFile) error {
	if cfg.Backup.S3CredentialMode != "" && !validImportAuth(cfg.Backup.S3CredentialMode) {
		return fmt.Errorf("backup.s3_credential_mode: unsupported value %q", cfg.Backup.S3CredentialMode)
	}
	if cfg.Restore.S3CredentialMode != "" && !validImportAuth(cfg.Restore.S3CredentialMode) {
		return fmt.Errorf("restore.s3_credential_mode: unsupported value %q", cfg.Restore.S3CredentialMode)
	}
	if cfg.Backup.Threads != nil && *cfg.Backup.Threads < 1 {
		return errors.New("backup.threads must be at least 1")
	}
	if cfg.Backup.MemoryBytes != nil && *cfg.Backup.MemoryBytes < 1 {
		return errors.New("backup.memory_bytes must be positive")
	}
	if _, err := configuredDuration(cfg.Backup.LockWait, "backup.lock_wait", 0); err != nil {
		return err
	}
	if _, err := configuredDuration(cfg.Backup.LockLease, "backup.lock_lease", 10*time.Minute); err != nil {
		return err
	}
	return nil
}

func validImportAuth(value string) bool {
	switch value {
	case "auto", "implicit", "specified", "served":
		return true
	default:
		return false
	}
}

func applyResourceCaps(threads int, memoryBytes int64) {
	if threads > 0 {
		runtime.GOMAXPROCS(threads)
	}
	if memoryBytes > 0 {
		debug.SetMemoryLimit(memoryBytes)
	}
}
