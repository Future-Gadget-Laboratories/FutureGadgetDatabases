// Copyright 2026 Future Gadget Laboratories
// Use of this source code is governed by the Apache License, Version 2.0
// that can be found in the LICENSE file.

package release

import (
	"testing"

	"github.com/cockroachdb/cockroach/pkg/util/version"
)

func TestParseTag(t *testing.T) {
	tests := []struct {
		name string
		tag  string
		want Release
		ok   bool
	}{
		{name: "fix line", tag: "v23.2.15-fgdb.7", want: Release{Line: FixLine, Fix: 7}, ok: true},
		{name: "provisional feature line", tag: "v23.2.15-fgdb-v1.2.3", want: Release{Line: FeatureLine, Major: 1, Minor: 2, Patch: 3}, ok: true},
		{name: "provisional release candidate", tag: "v23.2.15-fgdb-v1.0.0-rc.2", want: Release{Line: FeatureLine, Major: 1, Patch: 0, Prerelease: 2}, ok: true},
		{name: "development build", tag: "v23.2.15-fgdb.8-dev", ok: false},
		{name: "missing FGDb prefix", tag: "v23.2.15", ok: false},
		{name: "zero fix number", tag: "v23.2.15-fgdb.0", ok: false},
		{name: "missing CRDB-shaped prefix", tag: "fgdb-v1.2.3", ok: false},
		{name: "leading zero", tag: "v23.2.15-fgdb-v01.2.3", ok: false},
		{name: "maximum component", tag: "v23.2.15-fgdb-v2147483647.2147483647.2147483647-rc.2147483647", want: Release{Line: FeatureLine, Major: 2147483647, Minor: 2147483647, Patch: 2147483647, Prerelease: 2147483647}, ok: true},
		{name: "fix component exceeds int32", tag: "v23.2.15-fgdb.2147483648", ok: false},
		{name: "major exceeds int32", tag: "v23.2.15-fgdb-v2147483648.0.0", ok: false},
		{name: "minor exceeds int32", tag: "v23.2.15-fgdb-v0.2147483648.0", ok: false},
		{name: "patch exceeds int32", tag: "v23.2.15-fgdb-v0.0.2147483648", ok: false},
		{name: "rc exceeds int32", tag: "v23.2.15-fgdb-v0.0.0-rc.2147483648", ok: false},
		{name: "overflow", tag: "v23.2.15-fgdb-v18446744073709551616.0.0", ok: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseTag(tc.tag)
			if (err == nil) != tc.ok {
				t.Fatalf("ParseTag(%q) error = %v, want success %t", tc.tag, err, tc.ok)
			}
			if tc.ok && got != tc.want {
				t.Errorf("ParseTag(%q) = %+v, want %+v", tc.tag, got, tc.want)
			}
		})
	}
}

func TestBuildTagsAreCRDBVersions(t *testing.T) {
	for _, tag := range []string{
		"v23.2.15-fgdb.7",
		"v23.2.15-fgdb-v1.0.0",
		"v23.2.15-fgdb-v1.0.0-rc.1",
	} {
		if _, err := version.Parse(tag); err != nil {
			t.Errorf("version.Parse(%q) = %v, want valid CRDB-shaped version", tag, err)
		}
	}
}
