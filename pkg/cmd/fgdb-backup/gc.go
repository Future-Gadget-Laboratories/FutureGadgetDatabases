// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

package main

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const defaultReportedGCTTL = 14400

// rangeDefaultZone is the crdb_internal.zones target for the cluster default.
const rangeDefaultZone = "RANGE default"

// sqlAlter is the prefix of an ALTER statement that names its target next.
const sqlAlter = "ALTER "

var gcTTLPattern = regexp.MustCompile(`(?i)gc\.ttlseconds\s*=\s*([0-9]+)`)

func parseGCTTL(sql string) (int, bool) {
	m := gcTTLPattern.FindStringSubmatch(sql)
	if m == nil {
		return 0, false
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, false
	}
	return n, true
}

// gcBudget is the time left to read a snapshot before garbage collection
// can remove it. deadline is asOf plus the smallest gc.ttlseconds in scope.
type gcBudget struct {
	minTTL   int
	object   string
	asOf     time.Time
	deadline time.Time
	margin   time.Duration
}

func (b gcBudget) remaining(now time.Time) time.Duration {
	return b.deadline.Sub(now)
}

func (b gcBudget) exceeded(now time.Time) bool {
	return !now.Add(b.margin).Before(b.deadline)
}

func (b gcBudget) message() string {
	return fmt.Sprintf(`the snapshot at %s can be read until %s because gc.ttlseconds is %d on %s. That leaves less than the safety margin of %s, so this backup would outrun garbage collection.

Rows at that timestamp can disappear once the TTL passes, and the read then fails. On a new cluster the default gc.ttlseconds is %d seconds (4 hours). Clusters often set a shorter value. Raise it for the backup window, for example:

  ALTER DATABASE <db> CONFIGURE ZONE USING gc.ttlseconds = 86400;

Pass --extend-gc-ttl=12h to raise the TTL for this run and put the old value back afterwards. The revert statements are printed when the run starts. If the process is killed before it finishes, the higher value can stay set until you run those statements.`,
		b.asOf.Format(time.RFC3339Nano),
		b.deadline.Format(time.RFC3339Nano),
		b.minTTL,
		b.object,
		b.margin,
		defaultReportedGCTTL,
	)
}

func newBudget(asOf time.Time, minTTL int, object string, margin time.Duration) gcBudget {
	return gcBudget{
		minTTL:   minTTL,
		object:   object,
		asOf:     asOf,
		deadline: asOf.Add(time.Duration(minTTL) * time.Second),
		margin:   margin,
	}
}

type gcChange struct {
	Object string
	Apply  string
	Revert string
}

// planGCTTLRaises decides which zone objects to raise. A table that sets its
// own gc.ttlseconds below the target is raised on its own. Each database whose
// effective TTL is still below the target is raised too, which covers tables
// that inherit the database setting. A database with no zone of its own gets a
// new zone that is discarded afterwards.
func planGCTTLRaises(desired int, databases []string, zones []zoneRow) []gcChange {
	if desired <= 0 {
		return nil
	}
	def, dbZone := indexZoneTTL(zones)
	changes := tableTTLChanges(desired, zones)
	return append(changes, databaseTTLChanges(desired, databases, def, dbZone)...)
}

func indexZoneTTL(zones []zoneRow) (int, map[string]zoneRow) {
	def := 0
	dbZone := map[string]zoneRow{}
	for _, z := range zones {
		switch z.Level {
		case "range":
			def = z.Effective
		case "database":
			dbZone[z.Database] = z
		}
	}
	return def, dbZone
}

func tableTTLChanges(desired int, zones []zoneRow) []gcChange {
	var changes []gcChange
	for _, z := range zones {
		if z.Level != "table" {
			continue
		}
		own, hasOwn := parseGCTTL(z.RawSQL)
		if !hasOwn || own >= desired {
			continue
		}
		changes = append(changes, ttlNumberChange(z, desired, own))
	}
	return changes
}

func ttlNumberChange(z zoneRow, desired, own int) gcChange {
	prefix := alterPrefix(z)
	return gcChange{
		Object: z.Object,
		Apply:  prefix + fmt.Sprintf(" CONFIGURE ZONE USING gc.ttlseconds = %d", desired),
		Revert: prefix + fmt.Sprintf(" CONFIGURE ZONE USING gc.ttlseconds = %d", own),
	}
}

func databaseTTLChanges(desired int, databases []string, def int, dbZone map[string]zoneRow) []gcChange {
	var changes []gcChange
	for _, database := range databases {
		z, ok := dbZone[database]
		change, apply := databaseTTLChange(database, z, ok, def, desired)
		if apply {
			changes = append(changes, change)
		}
	}
	return changes
}

func databaseEffective(z zoneRow, ok bool, def int) int {
	if ok && z.Effective > 0 {
		return z.Effective
	}
	return def
}

func databaseTTLChange(database string, z zoneRow, ok bool, def, desired int) (gcChange, bool) {
	eff := databaseEffective(z, ok, def)
	if eff == 0 || eff >= desired {
		return gcChange{}, false
	}
	if !ok {
		return newDatabaseTTL(database, desired), true
	}
	if n, has := parseGCTTL(z.RawSQL); has {
		if n >= desired {
			return gcChange{}, false
		}
		return ttlNumberChange(z, desired, n), true
	}
	prefix := alterPrefix(z)
	return gcChange{
		Object: z.Object,
		Apply:  prefix + fmt.Sprintf(" CONFIGURE ZONE USING gc.ttlseconds = %d", desired),
		Revert: prefix + " CONFIGURE ZONE USING gc.ttlseconds = COPY FROM PARENT",
	}, true
}

func newDatabaseTTL(database string, desired int) gcChange {
	obj := "DATABASE " + quoteIdent(database)
	return gcChange{
		Object: obj,
		Apply:  sqlAlter + obj + fmt.Sprintf(" CONFIGURE ZONE USING gc.ttlseconds = %d", desired),
		Revert: sqlAlter + obj + " CONFIGURE ZONE DISCARD",
	}
}

func alterPrefix(z zoneRow) string {
	raw := strings.TrimSpace(z.RawSQL)
	if i := strings.Index(strings.ToUpper(raw), " CONFIGURE ZONE"); i > 0 {
		return strings.TrimSpace(raw[:i])
	}
	switch z.Level {
	case "database":
		return "ALTER DATABASE " + quoteIdent(z.Database)
	case "table":
		return "ALTER TABLE " + qualified(z.Database, z.Schema, z.Table)
	case "index":
		return "ALTER INDEX " + qualified(z.Database, z.Schema, z.Table) + "@" + quoteIdent(z.Index)
	default:
		return sqlAlter + z.Object
	}
}

type zoneRow struct {
	Level     string
	Object    string
	Database  string
	Schema    string
	Table     string
	Index     string
	Partition string
	RawSQL    string
	FullSQL   string
	Effective int
}

func (z zoneRow) key() string {
	return z.Level + ":" + z.Object
}
