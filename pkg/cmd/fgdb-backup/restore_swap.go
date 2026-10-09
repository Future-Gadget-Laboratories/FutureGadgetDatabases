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
	suffix := time.Now().UTC().Format("20060102t150405")
	temp := strings.ToLower(name + "__fgdb_restore_" + suffix)
	old := strings.ToLower(name + "__fgdb_old_" + suffix)
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
	if len(plan.Swap) == 0 {
		return res, fmt.Errorf("swap restore has no database eligible for swapping")
	}
	names := map[string]string{}
	oldCopies := map[string]string{}
	for _, oldName := range plan.Swap {
		tempName, err := swapNames(ctx, db, oldName)
		if err != nil {
			return res, err
		}
		names[oldName] = tempName
		oldCopies[oldName] = strings.ToLower(oldName + "__fgdb_old_" + time.Now().UTC().Format("20060102t150405"))
	}
	for oldName := range selected {
		if _, ok := names[oldName]; !ok {
			names[oldName] = oldName
		}
	}
	tempBundle := remapBundleNames(bundle, names)
	tempSelected := make(map[string]bool, len(names))
	for _, tempName := range names {
		tempSelected[tempName] = true
	}
	if err := restoreIntoEmpty(ctx, db, tempBundle, tempSelected, opt, &res); err != nil {
		return res, err
	}
	if _, err := db.exec(ctx, "BEGIN"); err != nil {
		return res, fmt.Errorf("begin database swap: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_, _ = db.exec(context.Background(), "ROLLBACK")
		}
	}()
	for oldName, tempName := range names {
		oldCopy := oldCopies[oldName]
		if _, err := db.exec(ctx, "ALTER DATABASE "+quoteIdent(oldName)+" RENAME TO "+quoteIdent(oldCopy)); err != nil {
			return res, fmt.Errorf("rename old database for swap: %w", err)
		}
		if _, err := db.exec(ctx, "ALTER DATABASE "+quoteIdent(tempName)+" RENAME TO "+quoteIdent(oldName)); err != nil {
			return res, fmt.Errorf("rename restored database for swap: %w", err)
		}
	}
	if _, err := db.exec(ctx, "COMMIT"); err != nil {
		return res, fmt.Errorf("commit database swap: %w", err)
	}
	committed = true
	for oldName, oldCopy := range oldCopies {
		res.Warnings = append(res.Warnings, "the previous database "+oldName+" was kept as "+oldCopy)
	}
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

func remapBundleNames(bundle restoreBundle, names map[string]string) restoreBundle {
	out := bundle
	out.manifest = appendManifestNames(bundle.manifest, names)
	out.objects = appendObjectsNames(bundle.objects, names)
	return out
}

func appendManifestNames(manifest Manifest, names map[string]string) Manifest {
	out := manifest
	out.Databases = append([]string(nil), manifest.Databases...)
	for i, name := range out.Databases {
		if replacement, ok := names[name]; ok {
			out.Databases[i] = replacement
		}
	}
	for i := range out.Tables {
		if replacement, ok := names[out.Tables[i].Database]; ok {
			out.Tables[i].Database = replacement
		}
	}
	return out
}

func appendObjectsNames(objects ObjectsFile, names map[string]string) ObjectsFile {
	out := objects
	out.Databases = append([]NamedSQL(nil), objects.Databases...)
	for i := range out.Databases {
		if replacement, ok := names[out.Databases[i].Name]; ok {
			out.Databases[i].Name = replacement
			out.Databases[i].SQL = "CREATE DATABASE " + quoteIdent(replacement) + ";"
		}
	}
	out.Statements = append([]Statement(nil), objects.Statements...)
	for oldName, newName := range names {
		out = remapObjectsOnce(out, oldName, newName)
	}
	return out
}

func remapObjectsOnce(objects ObjectsFile, oldName, newName string) ObjectsFile {
	out := objects
	out.Statements = remapStatements(out.Statements, oldName, newName)
	out.SequenceValues = append([]SequenceValue(nil), objects.SequenceValues...)
	for i := range out.SequenceValues {
		if out.SequenceValues[i].Database != oldName {
			continue
		}
		out.SequenceValues[i].Database = newName
		out.SequenceValues[i].Object = rewriteDatabaseName(out.SequenceValues[i].Object, oldName, newName)
		out.SequenceValues[i].SQL = sequenceRestoreSQLForDatabase(out.SequenceValues[i])
	}
	out.Grants = rewriteList(out.Grants, oldName, newName)
	out.Zones = append([]ZoneStatement(nil), objects.Zones...)
	for i := range out.Zones {
		if out.Zones[i].Database != oldName {
			continue
		}
		out.Zones[i].Database = newName
		out.Zones[i].Object = rewriteDatabaseName(out.Zones[i].Object, oldName, newName)
		out.Zones[i].SQL = rewriteDatabaseName(out.Zones[i].SQL, oldName, newName)
	}
	return out
}

func sequenceRestoreSQLForDatabase(seq SequenceValue) string {
	parts := splitQualified(seq.Object)
	if len(parts) < 2 {
		return seq.SQL
	}
	target := quoteLiteral(qualified(seq.Database, parts[len(parts)-2], parts[len(parts)-1]))
	return fmt.Sprintf("SELECT setval(%s::REGCLASS, %d, %t);", target, seq.LastValue, seq.IsCalled)
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
