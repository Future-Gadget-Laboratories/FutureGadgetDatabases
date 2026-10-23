// Copyright 2026 Future Gadget Laboratories
// Use of this source code is governed by the Apache License, Version 2.0
// that can be found in the LICENSE file.

// Package gate defines the FGDb feature-setting and disabled-path contract.
package gate

import (
	"errors"
	"fmt"
)

const settingPrefix = "fgdb."
const settingSuffix = ".enabled"

// ErrInvalidFeatureName indicates that a feature name cannot be used in a
// cluster-setting name. Feature names use lowercase ASCII letters, digits, and
// underscores, and must start with a lowercase letter.
var ErrInvalidFeatureName = errors.New("invalid FGDb feature name")

// SettingName returns the cluster-setting name for feature, such as
// "fgdb.backup.enabled". It returns ErrInvalidFeatureName for an invalid name.
func SettingName(feature string) (string, error) {
	if len(feature) == 0 || feature[0] < 'a' || feature[0] > 'z' {
		return "", fmt.Errorf("%w: %q", ErrInvalidFeatureName, feature)
	}
	for i := 1; i < len(feature); i++ {
		c := feature[i]
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '_' {
			return "", fmt.Errorf("%w: %q", ErrInvalidFeatureName, feature)
		}
	}
	return settingPrefix + feature + settingSuffix, nil
}

// Run calls the stock path while disabled and the FGDb path while enabled.
// The caller supplies both paths so the disabled branch preserves its existing
// result and error, including SQLSTATE when returned by that path. A nil
// callback for the selected branch returns an error.
func Run(enabled bool, stockPath, fgdbPath func() error) error {
	path := stockPath
	if enabled {
		path = fgdbPath
	}
	if path == nil {
		return errors.New("selected FGDb gate path is nil")
	}
	return path()
}
