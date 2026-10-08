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
	def := 0
	dbZone := map[string]zoneRow{}
	var changes []gcChange
	for _, z := range zones {
		switch z.Level {
		case "range":
			def = z.Effective
		case "database":
			dbZone[z.Database] = z
		case "table":
			own, hasOwn := parseGCTTL(z.RawSQL)
			if !hasOwn || own >= desired {
				continue
			}
			prefix := alterPrefix(z)
			changes = append(changes, gcChange{
				Object: z.Object,
				Apply:  prefix + fmt.Sprintf(" CONFIGURE ZONE USING gc.ttlseconds = %d", desired),
				Revert: prefix + fmt.Sprintf(" CONFIGURE ZONE USING gc.ttlseconds = %d", own),
			})
		}
	}
	for _, database := range databases {
		z, ok := dbZone[database]
		eff := def
		if ok && z.Effective > 0 {
			eff = z.Effective
		}
		if eff == 0 || eff >= desired {
			continue
		}
		if !ok {
			obj := "DATABASE " + quoteIdent(database)
			changes = append(changes, gcChange{
				Object: obj,
				Apply:  "ALTER " + obj + fmt.Sprintf(" CONFIGURE ZONE USING gc.ttlseconds = %d", desired),
				Revert: "ALTER " + obj + " CONFIGURE ZONE DISCARD",
			})
			continue
		}
		prefix := alterPrefix(z)
		if n, has := parseGCTTL(z.RawSQL); has {
			if n >= desired {
				continue
			}
			changes = append(changes, gcChange{
				Object: z.Object,
				Apply:  prefix + fmt.Sprintf(" CONFIGURE ZONE USING gc.ttlseconds = %d", desired),
				Revert: prefix + fmt.Sprintf(" CONFIGURE ZONE USING gc.ttlseconds = %d", n),
			})
			continue
		}
		changes = append(changes, gcChange{
			Object: z.Object,
			Apply:  prefix + fmt.Sprintf(" CONFIGURE ZONE USING gc.ttlseconds = %d", desired),
			Revert: prefix + " CONFIGURE ZONE USING gc.ttlseconds = COPY FROM PARENT",
		})
	}
	return changes
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
		return "ALTER " + z.Object
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
