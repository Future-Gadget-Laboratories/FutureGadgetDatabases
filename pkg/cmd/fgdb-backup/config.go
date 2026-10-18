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
	"strconv"
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
	Lock                 *bool  `yaml:"lock"`
	LockWait             string `yaml:"lock_wait"`
	LockLease            string `yaml:"lock_lease"`
	Threads              *int   `yaml:"threads"`
	MemoryBytes          *int64 `yaml:"memory_bytes"`
	FileMode             string `yaml:"file_mode"`
	AllowUnsafeOverwrite *bool  `yaml:"allow_unsafe_overwrite"`
	S3CredentialMode     string `yaml:"s3_credential_mode"`
}

type restoreConfigValues struct {
	SwapRestore          *bool  `yaml:"swap_restore"`
	InPlace              *bool  `yaml:"in_place"`
	Retention            *int   `yaml:"retention"`
	TestingMode          *bool  `yaml:"testing_mode"`
	Threads              *int   `yaml:"threads"`
	MemoryBytes          *int64 `yaml:"memory_bytes"`
	AllowUnsafeOverwrite *bool  `yaml:"allow_unsafe_overwrite"`
	S3CredentialMode     string `yaml:"s3_credential_mode"`
}

func configPath(args []string) (string, error) {
	for i, arg := range args {
		if arg == "-config" || strings.HasPrefix(arg, "-config=") {
			return "", errors.New("use --config; the single-dash -config form is not supported")
		}
		if arg == "--config" && i+1 < len(args) {
			return args[i+1], nil
		}
		if strings.HasPrefix(arg, "--config=") {
			return strings.TrimPrefix(arg, "--config="), nil
		}
	}
	return "", nil
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
	decoder := yaml.NewDecoder(strings.NewReader(string(body)))
	decoder.KnownFields(true)
	if err := decoder.Decode(&cfg); err != nil {
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

func configuredFileMode(value string) (os.FileMode, error) {
	if value == "" {
		return 0o640, nil
	}
	n, err := strconv.ParseUint(value, 8, 12)
	if err != nil || n == 0 {
		return 0, fmt.Errorf("backup.file_mode must be a non-zero octal mode")
	}
	return os.FileMode(n), nil
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
	if cfg.Backup.Threads != nil && *cfg.Backup.Threads < 0 {
		return errors.New("backup.threads must not be negative")
	}
	if cfg.Backup.MemoryBytes != nil && *cfg.Backup.MemoryBytes < 0 {
		return errors.New("backup.memory_bytes must not be negative")
	}
	if cfg.Restore.Threads != nil && *cfg.Restore.Threads < 0 {
		return errors.New("restore.threads must not be negative")
	}
	if cfg.Restore.MemoryBytes != nil && *cfg.Restore.MemoryBytes < 0 {
		return errors.New("restore.memory_bytes must not be negative")
	}
	if cfg.Restore.Retention != nil && *cfg.Restore.Retention < 0 {
		return errors.New("restore.retention must not be negative")
	}
	if _, err := configuredFileMode(cfg.Backup.FileMode); err != nil {
		return err
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
