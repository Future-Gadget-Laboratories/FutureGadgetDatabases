// Copyright 2026 Future Gadget Laboratories
// Use of this source code is governed by the Apache License, Version 2.0
// that can be found in the LICENSE file.

package release

import "testing"

func TestParseTag(t *testing.T) {
	tests := []struct {
		name string
		tag  string
		want Release
		ok   bool
	}{
		{name: "fix line", tag: "v23.2.15-fgdb.7", want: Release{Line: FixLine, Fix: 7}, ok: true},
		{name: "provisional feature line", tag: "fgdb-v1.2.3", want: Release{Line: FeatureLine, Major: 1, Minor: 2, Patch: 3}, ok: true},
		{name: "provisional release candidate", tag: "fgdb-v1.0.0-rc.2", want: Release{Line: FeatureLine, Major: 1, Patch: 0, Prerelease: 2}, ok: true},
		{name: "development build", tag: "v23.2.15-fgdb.8-dev", ok: false},
		{name: "missing FGDb prefix", tag: "v23.2.15", ok: false},
		{name: "zero fix number", tag: "v23.2.15-fgdb.0", ok: false},
		{name: "leading zero", tag: "fgdb-v01.2.3", ok: false},
		{name: "overflow", tag: "fgdb-v18446744073709551616.0.0", ok: false},
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
