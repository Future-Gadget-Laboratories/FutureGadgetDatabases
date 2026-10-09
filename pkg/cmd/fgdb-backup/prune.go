// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

package main

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

func pruneOldCopies(ctx context.Context, db *database, name string, keep int) ([]string, error) {
	if keep < 0 {
		return nil, fmt.Errorf("retention must not be negative")
	}
	rows, err := db.query(ctx, "SELECT database_name FROM [SHOW DATABASES]")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	prefix := strings.ToLower(name + "__fgdb_old_")
	var copies []string
	for rows.Next() {
		var database string
		if err := rows.Scan(&database); err != nil {
			return nil, err
		}
		if strings.HasPrefix(strings.ToLower(database), prefix) {
			copies = append(copies, database)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Sort(sort.Reverse(sort.StringSlice(copies)))
	if keep >= len(copies) {
		return nil, nil
	}
	var removed []string
	for _, database := range copies[keep:] {
		if _, err := db.exec(ctx, "DROP DATABASE "+quoteIdent(database)+" CASCADE"); err != nil {
			return removed, fmt.Errorf("drop old copy %s: %w", database, err)
		}
		removed = append(removed, database)
	}
	return removed, nil
}
