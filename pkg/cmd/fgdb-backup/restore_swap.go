// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

package main

import (
	"context"
	"fmt"
	"strings"
	"time"
)

func swapNames(ctx context.Context, db *database, name string) (string, error) {
	suffix := time.Now().UTC().Format("20060102T150405")
	temp := name + "__fgdb_restore_" + suffix
	old := name + "__fgdb_old_" + suffix
	if err := safeSegment(temp); err != nil {
		return "", err
	}
	if err := safeSegment(old); err != nil {
		return "", err
	}
	for _, candidate := range []string{temp, old} {
		exists, err := databaseExists(ctx, db, candidate)
		if err != nil {
			return "", err
		}
		if exists {
			return "", fmt.Errorf("swap name %s already exists; choose a later restore run", candidate)
		}
	}
	return temp, nil
}

func restoreAsideAndSwap(ctx context.Context, db *database, bundle restoreBundle, selected map[string]bool, opt RestoreOptions, res RestoreResult, plan RestorePlan) (RestoreResult, error) {
	if len(plan.Swap) != 1 || len(selected) != 1 {
		return res, fmt.Errorf("swap restore currently requires one selected database")
	}
	oldName := plan.Swap[0]
	tempName, err := swapNames(ctx, db, oldName)
	if err != nil {
		return res, err
	}
	tempBundle := remapBundle(bundle, oldName, tempName)
	tempSelected := map[string]bool{tempName: true}
	if err := restoreIntoEmpty(ctx, db, tempBundle, tempSelected, opt, &res); err != nil {
		return res, err
	}
	oldCopy := oldName + "__fgdb_old_" + time.Now().UTC().Format("20060102T150405")
	if _, err := db.exec(ctx, "BEGIN"); err != nil {
		return res, fmt.Errorf("begin database swap: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_, _ = db.exec(context.Background(), "ROLLBACK")
		}
	}()
	if _, err := db.exec(ctx, "ALTER DATABASE "+quoteIdent(oldName)+" RENAME TO "+quoteIdent(oldCopy)); err != nil {
		return res, fmt.Errorf("rename old database for swap: %w", err)
	}
	if _, err := db.exec(ctx, "ALTER DATABASE "+quoteIdent(tempName)+" RENAME TO "+quoteIdent(oldName)); err != nil {
		return res, fmt.Errorf("rename restored database for swap: %w", err)
	}
	if _, err := db.exec(ctx, "COMMIT"); err != nil {
		return res, fmt.Errorf("commit database swap: %w", err)
	}
	committed = true
	res.Warnings = append(res.Warnings, "the previous database was kept as "+oldCopy)
	return res, nil
}

func restoreIntoEmpty(ctx context.Context, db *database, bundle restoreBundle, selected map[string]bool, opt RestoreOptions, res *RestoreResult) error {
	if stopped, err := restoreSchema(ctx, db, bundle.objects, selected, res); stopped {
		return err
	}
	if err := prepareTargets(ctx, db, bundle.manifest, selected); err != nil {
		return err
	}
	warnings, err := loadSelected(ctx, db, bundle, selected, opt)
	if err != nil {
		return err
	}
	res.Warnings = append(res.Warnings, warnings...)
	_, err = finishRestore(ctx, db, bundle, selected, res)
	return err
}

func remapBundle(bundle restoreBundle, oldName, newName string) restoreBundle {
	out := bundle
	out.manifest = remapManifest(bundle.manifest, oldName, newName)
	out.objects = remapObjects(bundle.objects, oldName, newName)
	return out
}

func remapManifest(manifest Manifest, oldName, newName string) Manifest {
	out := manifest
	out.Databases = []string{newName}
	out.Name = newName
	for i := range out.Tables {
		out.Tables[i].Database = newName
	}
	return out
}

func remapObjects(objects ObjectsFile, oldName, newName string) ObjectsFile {
	out := objects
	out.Databases = nil
	for _, database := range objects.Databases {
		if database.Name != oldName {
			continue
		}
		out.Databases = append(out.Databases, NamedSQL{
			Name: newName,
			SQL:  "CREATE DATABASE " + quoteIdent(newName) + ";",
		})
	}
	out.Statements = remapStatements(objects.Statements, oldName, newName)
	out.SequenceValues = append([]SequenceValue(nil), objects.SequenceValues...)
	for i := range out.SequenceValues {
		out.SequenceValues[i].Database = newName
		out.SequenceValues[i].Object = rewriteDatabaseName(out.SequenceValues[i].Object, oldName, newName)
		out.SequenceValues[i].SQL = rewriteDatabaseName(out.SequenceValues[i].SQL, oldName, newName)
	}
	out.Grants = rewriteList(objects.Grants, oldName, newName)
	out.Zones = append([]ZoneStatement(nil), objects.Zones...)
	for i := range out.Zones {
		out.Zones[i].Database = newName
		out.Zones[i].Object = rewriteDatabaseName(out.Zones[i].Object, oldName, newName)
		out.Zones[i].SQL = rewriteDatabaseName(out.Zones[i].SQL, oldName, newName)
	}
	return out
}

func remapStatements(stmts []Statement, oldName, newName string) []Statement {
	out := append([]Statement(nil), stmts...)
	for i := range out {
		if out[i].Database == oldName {
			out[i].Database = newName
		}
		out[i].SQL = rewriteDatabaseName(out[i].SQL, oldName, newName)
		out[i].Object = rewriteDatabaseName(out[i].Object, oldName, newName)
	}
	return out
}

func rewriteList(values []string, oldName, newName string) []string {
	out := append([]string(nil), values...)
	for i := range out {
		out[i] = rewriteDatabaseName(out[i], oldName, newName)
	}
	return out
}

func rewriteDatabaseName(value, oldName, newName string) string {
	value = strings.ReplaceAll(value, oldName+".", newName+".")
	value = strings.ReplaceAll(value, `"`+oldName+`".`, `"`+newName+`".`)
	return value
}
