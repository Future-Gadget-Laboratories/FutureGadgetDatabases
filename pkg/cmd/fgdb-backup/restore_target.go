// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

package main

import (
	"context"
	"fmt"
	"strings"
)

// guardRestoreTargets refuses a database that already has user objects unless
// --force is set. --force drops only objects that are in the backup, without
// CASCADE, so a table outside the backup is not emptied.
func guardRestoreTargets(ctx context.Context, db *database, objects ObjectsFile, selected map[string]bool, force bool) error {
	for _, database := range objects.Databases {
		if !selected[database.Name] {
			continue
		}
		exists, err := databaseExists(ctx, db, database.Name)
		if err != nil {
			return err
		}
		if !exists {
			continue
		}
		nonEmpty, err := databaseHasUserObjects(ctx, db, database.Name)
		if err != nil {
			return err
		}
		if !nonEmpty {
			continue
		}
		if !force {
			return fmt.Errorf("refusing to restore into non-empty database %s. A failed restore can leave a partial database; rerun with --force to drop and recreate only the objects in this backup", database.Name)
		}
		if err := dropBackupObjects(ctx, db, objects, database.Name); err != nil {
			return err
		}
	}
	return nil
}

func databaseExists(ctx context.Context, db *database, name string) (bool, error) {
	var n int
	err := db.queryRow(ctx, "SELECT count(*) FROM [SHOW DATABASES] WHERE database_name = $1", name).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("check database %s: %w", name, err)
	}
	return n > 0, nil
}

func databaseHasUserObjects(ctx context.Context, db *database, name string) (bool, error) {
	if err := db.use(ctx, name); err != nil {
		return false, err
	}
	var tables int
	err := db.queryRow(ctx, `
SELECT count(*) FROM [SHOW TABLES]
WHERE schema_name NOT IN ('pg_catalog', 'information_schema', 'crdb_internal', 'pg_extension')`).Scan(&tables)
	if err != nil {
		return false, fmt.Errorf("list objects in %s: %w", name, err)
	}
	if tables > 0 {
		return true, nil
	}
	var types int
	if err := db.queryRow(ctx, "SELECT count(*) FROM [SHOW TYPES]").Scan(&types); err != nil {
		return false, fmt.Errorf("list types in %s: %w", name, err)
	}
	if types > 0 {
		return true, nil
	}
	var routines int
	q := `SELECT count(*) FROM (
		SELECT function_name FROM crdb_internal.create_function_statements WHERE database_name = $1
		UNION ALL
		SELECT procedure_name FROM crdb_internal.create_procedure_statements WHERE database_name = $1
	) AS routines`
	if err := db.queryRow(ctx, q, name).Scan(&routines); err != nil {
		return false, fmt.Errorf("list routines in %s: %w", name, err)
	}
	return routines > 0, nil
}

func dropBackupObjects(ctx context.Context, db *database, objects ObjectsFile, database string) error {
	if err := db.use(ctx, database); err != nil {
		return err
	}
	// An outside foreign key makes DROP TABLE fail. Check that before dropping
	// views and functions, so a refusal leaves those objects in place.
	if err := outsideForeignKeys(ctx, db, objects, database); err != nil {
		return err
	}
	if err := dropKinds(ctx, db, objects, database, []string{"view", "materialized_view"}, dropViewSQL, true); err != nil {
		return err
	}
	if err := dropRoutines(ctx, db, objects, database); err != nil {
		return err
	}
	if err := dropTables(ctx, db, objects, database); err != nil {
		return err
	}
	if err := dropKinds(ctx, db, objects, database, []string{"sequence"}, dropSequenceSQL, false); err != nil {
		return err
	}
	return dropKinds(ctx, db, objects, database, []string{"type"}, dropTypeSQL, false)
}

// outsideForeignKeys refuses --force when a table that is not in the backup
// references one that is. DROP TABLE without CASCADE would fail later, after
// views and functions were already gone.
func outsideForeignKeys(ctx context.Context, db *database, objects ObjectsFile, database string) error {
	backup := map[string]bool{}
	for _, st := range objects.Statements {
		if st.Database != database || st.Kind != "table" {
			continue
		}
		if key := relationKey(st.Object); key != "" {
			backup[key] = true
		}
	}
	rows, err := db.query(ctx, `
SELECT n.nspname, c.relname, rn.nspname, rc.relname
FROM pg_constraint AS f
JOIN pg_class AS c ON c.oid = f.conrelid
JOIN pg_namespace AS n ON n.oid = c.relnamespace
JOIN pg_class AS rc ON rc.oid = f.confrelid
JOIN pg_namespace AS rn ON rn.oid = rc.relnamespace
WHERE f.contype = 'f'
  AND n.nspname NOT IN ('pg_catalog', 'information_schema', 'crdb_internal', 'pg_extension')
  AND rn.nspname NOT IN ('pg_catalog', 'information_schema', 'crdb_internal', 'pg_extension')`)
	if err != nil {
		return fmt.Errorf("check foreign keys in %s: %w", database, err)
	}
	defer rows.Close()
	for rows.Next() {
		var childSchema, childName, parentSchema, parentName string
		if err := rows.Scan(&childSchema, &childName, &parentSchema, &parentName); err != nil {
			return err
		}
		parent := parentSchema + "." + parentName
		child := childSchema + "." + childName
		if backup[parent] && !backup[child] {
			return fmt.Errorf("refusing to restore into %s. %s references %s and is not in the backup. --force does not drop that table, and nothing in the backup was dropped", database, child, parent)
		}
	}
	return rows.Err()
}

func relationKey(object string) string {
	parts := splitQualified(strings.TrimSpace(object))
	if len(parts) >= 2 {
		return parts[len(parts)-2] + "." + parts[len(parts)-1]
	}
	if len(parts) == 1 {
		return parts[0]
	}
	return ""
}

func splitQualified(object string) []string {
	var parts []string
	var b strings.Builder
	inQuote := false
	for i := 0; i < len(object); i++ {
		c := object[i]
		if inQuote {
			if c == '"' {
				if i+1 < len(object) && object[i+1] == '"' {
					b.WriteByte('"')
					i++
					continue
				}
				inQuote = false
				continue
			}
			b.WriteByte(c)
			continue
		}
		switch c {
		case '"':
			inQuote = true
		case '.':
			parts = append(parts, b.String())
			b.Reset()
		default:
			b.WriteByte(c)
		}
	}
	if b.Len() > 0 || strings.HasSuffix(object, ".") {
		parts = append(parts, b.String())
	}
	return parts
}

func dropKinds(ctx context.Context, db *database, objects ObjectsFile, database string, kinds []string, sql func(Statement) (string, error), reverse bool) error {
	matched := matchingStatements(objects, database, kinds)
	if reverse {
		for i, j := 0, len(matched)-1; i < j; i, j = i+1, j-1 {
			matched[i], matched[j] = matched[j], matched[i]
		}
	}
	for _, st := range matched {
		stmt, err := sql(st)
		if err != nil {
			return fmt.Errorf("drop %s: %w", statementObject(st), err)
		}
		if _, err := db.exec(ctx, stmt); err != nil {
			return fmt.Errorf("drop %s: %w", statementObject(st), err)
		}
	}
	return nil
}

func matchingStatements(objects ObjectsFile, database string, kinds []string) []Statement {
	want := map[string]bool{}
	for _, kind := range kinds {
		want[kind] = true
	}
	var matched []Statement
	for _, st := range objects.Statements {
		if st.Database != database || !want[st.Kind] {
			continue
		}
		matched = append(matched, st)
	}
	return matched
}

func dropViewSQL(st Statement) (string, error) {
	kind := "VIEW"
	if st.Kind == "materialized_view" {
		kind = "MATERIALIZED VIEW"
	}
	if st.Object == "" {
		return "", fmt.Errorf("missing object name")
	}
	name, err := quotedObjectName(st.Object, 2)
	if err != nil {
		return "", err
	}
	return "DROP " + kind + " IF EXISTS " + name, nil
}

func dropSequenceSQL(st Statement) (string, error) {
	if st.Object == "" {
		return "", fmt.Errorf("missing object name")
	}
	name, err := quotedObjectName(st.Object, 2)
	if err != nil {
		return "", err
	}
	return "DROP SEQUENCE IF EXISTS " + name, nil
}

func dropTypeSQL(st Statement) (string, error) {
	if st.Object == "" {
		return "", fmt.Errorf("missing object name")
	}
	name, err := quotedObjectName(st.Object, 2)
	if err != nil {
		return "", err
	}
	return "DROP TYPE IF EXISTS " + name, nil
}

func dropRoutines(ctx context.Context, db *database, objects ObjectsFile, database string) error {
	for _, st := range objects.Statements {
		if st.Database != database || (st.Kind != "function" && st.Kind != "procedure") {
			continue
		}
		stmt, err := dropRoutineStatement(st.Kind, st.SQL)
		if err != nil {
			return fmt.Errorf("%s %s: %w", st.Kind, statementObject(st), err)
		}
		if _, err := db.exec(ctx, stmt); err != nil {
			return fmt.Errorf("drop %s %s: %w", st.Kind, statementObject(st), err)
		}
	}
	return nil
}

func dropTables(ctx context.Context, db *database, objects ObjectsFile, database string) error {
	var names []string
	for _, st := range objects.Statements {
		if st.Database != database || st.Kind != "table" || st.Object == "" {
			continue
		}
		name, err := quotedObjectName(st.Object, 2)
		if err != nil {
			return err
		}
		names = append(names, name)
	}
	if len(names) == 0 {
		return nil
	}
	// One statement drops foreign keys between these tables. CASCADE is not
	// used, so a table outside the backup that references one of them blocks
	// the drop instead of being emptied.
	_, err := db.exec(ctx, "DROP TABLE IF EXISTS "+strings.Join(names, ", "))
	if err != nil {
		return fmt.Errorf("drop tables in %s without cascading into unrelated tables: %w", database, err)
	}
	return nil
}

func quotedObjectName(object string, maxParts int) (string, error) {
	parts := splitQualified(strings.TrimSpace(object))
	if len(parts) == 0 {
		return "", fmt.Errorf("missing object name")
	}
	if maxParts > 0 && len(parts) > maxParts {
		parts = parts[len(parts)-maxParts:]
	}
	quoted := make([]string, len(parts))
	for i, part := range parts {
		if part == "" {
			return "", fmt.Errorf("invalid object name %q", object)
		}
		quoted[i] = quoteIdent(part)
	}
	return strings.Join(quoted, "."), nil
}
