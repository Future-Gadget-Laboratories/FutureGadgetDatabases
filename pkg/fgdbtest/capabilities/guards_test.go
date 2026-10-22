// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

package capabilities

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestEqualRows(t *testing.T) {
	for name, tc := range map[string]struct {
		got, want [][]string
		ok        bool
	}{
		"same":          {[][]string{{"1", "alpha"}}, [][]string{{"1", "alpha"}}, true},
		"wrong value":   {[][]string{{"1", "beta"}}, [][]string{{"1", "alpha"}}, false},
		"wrong count":   {[][]string{{"1"}}, [][]string{{"1"}, {"2"}}, false},
		"nil and empty": {nil, [][]string{}, true},
	} {
		t.Run(name, func(t *testing.T) {
			if got := equalRows(tc.got, tc.want); got != tc.ok {
				t.Fatalf("equalRows=%v, want %v", got, tc.ok)
			}
		})
	}
}

func TestBesideBinaryLib(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "cockroach")
	if _, err := os.Create(bin); err != nil {
		t.Fatal(err)
	}
	if _, err := besideBinaryLib(bin); err == nil {
		t.Fatal("missing lib passed")
	}
	lib := filepath.Join(root, "lib")
	if err := os.Mkdir(lib, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"libgeos.so", "libgeos_c.so"} {
		if _, err := os.Create(filepath.Join(lib, name)); err != nil {
			t.Fatal(err)
		}
	}
	got, err := besideBinaryLib(bin)
	if err != nil || got != lib {
		t.Fatalf("besideBinaryLib=%q, %v; want %q", got, err, lib)
	}
}

func TestImportFixtureHandler(t *testing.T) {
	w := httptest.NewRecorder()
	fixtureHandler([]importFixture{{format: "CSV", path: "/rows", body: "fixture"}})(w, httptest.NewRequest(http.MethodGet, "/rows", nil))
	if w.Code != http.StatusOK || w.Body.String() != "fixture" {
		t.Fatalf("fixture response status=%d body=%q", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	fixtureHandler(nil)(w, httptest.NewRequest(http.MethodGet, "/missing", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("missing fixture status=%d", w.Code)
	}
}
