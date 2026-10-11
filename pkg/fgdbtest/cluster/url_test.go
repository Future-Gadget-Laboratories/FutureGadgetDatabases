// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

package cluster

import (
	"strings"
	"testing"
)

func TestWorkloadURLHasNoDatabase(t *testing.T) {
	c := &Cluster{spec: fillSpec(Spec{Dir: t.TempDir(), Binary: "cockroach", Nodes: 3})}
	if err := c.layout(); err != nil {
		t.Fatal(err)
	}
	work, err := c.WorkloadURL(0)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(work, pgDB) {
		t.Fatalf("workload URL %s", work)
	}
	admin, err := c.SQLURL(1)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(admin, pgDB) || !strings.Contains(admin, "sslmode=disable") {
		t.Fatalf("admin URL %s", admin)
	}
}
