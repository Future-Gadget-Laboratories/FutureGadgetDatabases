// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

package invariants

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadHistoryDropsPartialTail(t *testing.T) {
	path := filepath.Join(t.TempDir(), "h.jsonl")
	line := "{\"client\":0,\"kind\":\"write\",\"value\":1,\"call_ns\":1,\"return_ns\":2,\"result\":\"ok\"}\n"
	body := line + "{\"client\":1,\"kind\":\"read\""
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	ops, err := ReadHistory(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(ops) != 1 || ops[0].Value != 1 {
		t.Fatalf("ops %+v", ops)
	}
}

func TestReadHistoryRejectsBadKind(t *testing.T) {
	path := filepath.Join(t.TempDir(), "h.jsonl")
	body := "{\"client\":0,\"kind\":\"drop\",\"value\":1,\"call_ns\":1,\"return_ns\":2,\"result\":\"ok\"}\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := ReadHistory(path)
	if err == nil || !strings.Contains(err.Error(), "kind") {
		t.Fatalf("got %v", err)
	}
}
