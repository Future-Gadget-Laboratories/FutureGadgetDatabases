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
	if err := dropKinds(ctx, db, objects, database, []string{"view", "materialized_view"}, dropViewSQL); err != nil {
		return err
	}
	if err := dropRoutines(ctx, db, objects, database); err != nil {
		return err
	}
	if err := dropTables(ctx, db, objects, database); err != nil {
		return err
	}
	if err := dropKinds(ctx, db, objects, database, []string{"sequence"}, dropSequenceSQL); err != nil {
		return err
	}
	return dropKinds(ctx, db, objects, database, []string{"type"}, dropTypeSQL)
}

func dropKinds(ctx context.Context, db *database, objects ObjectsFile, database string, kinds []string, sql func(Statement) (string, error)) error {
	want := map[string]bool{}
	for _, kind := range kinds {
		want[kind] = true
	}
	for _, st := range objects.Statements {
		if st.Database != database || !want[st.Kind] {
			continue
		}
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

func dropViewSQL(st Statement) (string, error) {
	kind := "VIEW"
	if st.Kind == "materialized_view" {
		kind = "MATERIALIZED VIEW"
	}
	if st.Object == "" {
		return "", fmt.Errorf("missing object name")
	}
	return "DROP " + kind + " IF EXISTS " + st.Object, nil
}

func dropSequenceSQL(st Statement) (string, error) {
	if st.Object == "" {
		return "", fmt.Errorf("missing object name")
	}
	name := st.Object
	if strings.Count(name, ".") > 1 {
		parts := strings.Split(name, ".")
		name = strings.Join(parts[len(parts)-2:], ".")
	}
	return "DROP SEQUENCE IF EXISTS " + name, nil
}

func dropTypeSQL(st Statement) (string, error) {
	if st.Object == "" {
		return "", fmt.Errorf("missing object name")
	}
	return "DROP TYPE IF EXISTS " + st.Object, nil
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
		names = append(names, st.Object)
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
