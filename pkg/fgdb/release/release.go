// Copyright 2026 Future Gadget Laboratories
// Use of this source code is governed by the Apache License, Version 2.0
// that can be found in the LICENSE file.

// Package release parses FGDb build tags.
package release

import (
	"fmt"
	"regexp"
	"strconv"
)

// Line identifies the release line encoded by a build tag.
type Line uint8

const (
	// FixLine is the existing v23.2.15-fgdb.N fixes-only line.
	FixLine Line = iota + 1
	// FeatureLine is the provisional FGDb-owned version line.
	FeatureLine
)

// Release is the FGDb release identity parsed from a build tag.
// Fix is the patch number for FixLine. Major, Minor, and Patch are the
// provisional N1 semver components for FeatureLine. Prerelease is an RC number
// when present; zero means a final release.
type Release struct {
	Line       Line
	Fix        uint64
	Major      uint64
	Minor      uint64
	Patch      uint64
	Prerelease uint64
}

var (
	fixTagRE     = regexp.MustCompile(`^v23\.2\.15-fgdb\.([1-9][0-9]*)$`)
	featureTagRE = regexp.MustCompile(`^fgdb-v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-rc\.([1-9][0-9]*))?$`)
)

// ParseTag parses an FGDb build tag. The feature-line grammar is provisional
// N1 and must be revisited when the naming decision is made. Non-FGDb tags,
// development tags, and malformed or non-canonical tags return an error.
func ParseTag(tag string) (Release, error) {
	if m := fixTagRE.FindStringSubmatch(tag); m != nil {
		fix, err := parseNumber(m[1])
		if err != nil {
			return Release{}, err
		}
		return Release{Line: FixLine, Fix: fix}, nil
	}
	if m := featureTagRE.FindStringSubmatch(tag); m != nil {
		major, err := parseNumber(m[1])
		if err != nil {
			return Release{}, err
		}
		minor, err := parseNumber(m[2])
		if err != nil {
			return Release{}, err
		}
		patch, err := parseNumber(m[3])
		if err != nil {
			return Release{}, err
		}
		var prerelease uint64
		if m[4] != "" {
			prerelease, err = parseNumber(m[4])
			if err != nil {
				return Release{}, err
			}
		}
		return Release{
			Line: FeatureLine, Major: major, Minor: minor, Patch: patch,
			Prerelease: prerelease,
		}, nil
	}
	return Release{}, fmt.Errorf("invalid or unsupported FGDb build tag %q", tag)
}

func parseNumber(s string) (uint64, error) {
	n, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid FGDb version number %q: %w", s, err)
	}
	return n, nil
}
