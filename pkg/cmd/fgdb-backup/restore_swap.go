// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

package main

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

func swapNames(ctx context.Context, db *database, name string) (string, string, error) {
	suffix := strings.ToLower(strings.ReplaceAll(newBackupID(time.Now()), ".", ""))
	temp := strings.ToLower(name + "__fgdb_restore_" + suffix)
	old := strings.ToLower(name + "__fgdb_old_" + suffix)
	if err := safeSegment(temp); err != nil {
		return "", "", err
	}
	if err := safeSegment(old); err != nil {
		return "", "", err
	}
	for _, candidate := range []string{temp, old} {
		exists, err := databaseExists(ctx, db, candidate)
		if err != nil {
			return "", "", err
		}
		if exists {
			return "", "", fmt.Errorf("swap name %s already exists; choose a later restore run", candidate)
		}
	}
	return temp, old, nil
}

func restoreAsideAndSwap(ctx context.Context, db *database, bundle restoreBundle, selected map[string]bool, opt RestoreOptions, res RestoreResult, plan RestorePlan) (RestoreResult, error) {
	if len(plan.Swap) == 0 {
		return res, fmt.Errorf("swap restore has no database eligible for swapping")
	}
	names := map[string]string{}
	oldCopies := map[string]string{}
	for _, oldName := range plan.Swap {
		tempName, oldCopy, err := swapNames(ctx, db, oldName)
		if err != nil {
			return res, err
		}
		names[oldName] = tempName
		oldCopies[oldName] = oldCopy
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
		if cleanupErr := cleanupSwapCopies(ctx, db, names); cleanupErr != nil {
			return res, fmt.Errorf("%w; temporary restore databases remain: %s (cleanup failed: %v)", err, strings.Join(tempNames(names), ", "), cleanupErr)
		}
		return res, fmt.Errorf("%w; temporary restore databases were removed", err)
	}
	if _, err := db.exec(ctx, "BEGIN"); err != nil {
		return res, fmt.Errorf("begin database swap: %w", err)
	}
	for oldName, tempName := range names {
		oldCopy := oldCopies[oldName]
		if _, err := db.exec(ctx, "ALTER DATABASE "+quoteIdent(oldName)+" RENAME TO "+quoteIdent(oldCopy)); err != nil {
			return rollbackSwap(ctx, db, names, res, fmt.Errorf("rename old database for swap: %w", err))
		}
		if _, err := db.exec(ctx, "ALTER DATABASE "+quoteIdent(tempName)+" RENAME TO "+quoteIdent(oldName)); err != nil {
			return rollbackSwap(ctx, db, names, res, fmt.Errorf("rename restored database for swap: %w", err))
		}
	}
	if _, err := db.exec(ctx, "COMMIT"); err != nil {
		return rollbackSwap(ctx, db, names, res, fmt.Errorf("commit database swap: %w", err))
	}
	for oldName, oldCopy := range oldCopies {
		res.Warnings = append(res.Warnings, "the previous database "+oldName+" was kept as "+oldCopy)
		if opt.Retention > 0 {
			removed, err := pruneOldCopies(ctx, db, oldName, opt.Retention, false)
			if err != nil {
				res.Warnings = append(res.Warnings, "old-copy retention could not prune "+oldName+": "+err.Error())
			} else if len(removed) > 0 {
				res.Warnings = append(res.Warnings, "pruned old database copies: "+strings.Join(removed, ", "))
			}
		}
	}
	return res, nil
}

func rollbackSwap(ctx context.Context, db *database, names map[string]string, res RestoreResult, cause error) (RestoreResult, error) {
	_, _ = db.exec(context.Background(), "ROLLBACK")
	if err := cleanupSwapCopies(ctx, db, names); err != nil {
		return res, fmt.Errorf("%w; temporary restore databases remain: %s (cleanup failed: %v)", cause, strings.Join(tempNames(names), ", "), err)
	}
	return res, fmt.Errorf("%w; temporary restore databases were removed", cause)
}

func tempNames(names map[string]string) []string {
	out := make([]string, 0, len(names))
	for oldName, name := range names {
		if oldName == name {
			continue
		}
		out = append(out, name)
	}
	return out
}

func cleanupSwapCopies(ctx context.Context, db *database, names map[string]string) error {
	var first error
	for oldName, temp := range names {
		if oldName == temp {
			continue
		}
		exists, err := databaseExists(ctx, db, temp)
		if err != nil {
			if first == nil {
				first = err
			}
			continue
		}
		if !exists {
			continue
		}
		if _, err := db.exec(ctx, "DROP DATABASE "+quoteIdent(temp)+" CASCADE"); err != nil && first == nil {
			first = err
		}
	}
	return first
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
	var out strings.Builder
	last := 0
	previous := ""
	for i := 0; i < len(value); {
		end, raw, quoted, ok := nextSQLName(value, i)
		if !ok {
			i = nextSQLPosition(i, end)
			continue
		}
		if rewriteNameAt(&out, rewriteNameOptions{
			value: value, last: last, start: i, end: end, raw: raw,
			quoted: quoted, previous: previous, oldName: oldName, newName: newName,
		}) {
			last = end
		}
		previous = previousSQLName(raw, quoted)
		i = end
	}
	out.WriteString(value[last:])
	return out.String()
}

func nextSQLPosition(start, end int) int {
	if end > start {
		return end
	}
	return start + 1
}

type rewriteNameOptions struct {
	value                      string
	last, start, end           int
	raw                        string
	quoted                     bool
	previous, oldName, newName string
}

func rewriteNameAt(out *strings.Builder, opt rewriteNameOptions) bool {
	if !isDatabaseName(opt.raw, opt.value, opt.end, opt.previous, opt.oldName) {
		return false
	}
	out.WriteString(opt.value[opt.last:opt.start])
	if opt.quoted {
		out.WriteString(quoteIdent(opt.newName))
	} else {
		out.WriteString(opt.newName)
	}
	return true
}

func isDatabaseName(raw, value string, end int, previous, oldName string) bool {
	next := skipSQLSpace(value, end)
	return strings.EqualFold(raw, oldName) && ((next < len(value) && value[next] == '.') || previous == "DATABASE")
}

func previousSQLName(raw string, quoted bool) string {
	if quoted {
		return ""
	}
	return strings.ToUpper(raw)
}

func nextSQLName(sql string, i int) (end int, name string, quoted, ok bool) {
	if i >= len(sql) {
		return i, "", false, false
	}
	switch sql[i] {
	case '\'', '-':
		return skipSQLLiteralOrComment(sql, i)
	case '"':
		end = skipSQLQuoted(sql, i)
		if end == i {
			return i, "", false, false
		}
		return end, strings.ReplaceAll(sql[i+1:end-1], `""`, `"`), true, true
	case '$':
		if end = skipSQLDollarQuote(sql, i); end != i {
			return end, "", false, false
		}
	}
	r, size := utf8.DecodeRuneInString(sql[i:])
	if !unicode.IsLetter(r) && r != '_' {
		return i, "", false, false
	}
	end = i + size
	for end < len(sql) {
		r, size = utf8.DecodeRuneInString(sql[end:])
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' && r != '$' {
			break
		}
		end += size
	}
	return end, sql[i:end], false, true
}

func skipSQLSpace(sql string, i int) int {
	for i < len(sql) && (sql[i] == ' ' || sql[i] == '\t' || sql[i] == '\n' || sql[i] == '\r') {
		i++
	}
	return i
}

func skipSQLQuoted(sql string, i int) int {
	for i := i + 1; i < len(sql); i++ {
		if sql[i] != '"' {
			continue
		}
		if i+1 < len(sql) && sql[i+1] == '"' {
			i++
			continue
		}
		return i + 1
	}
	return len(sql)
}

func skipSQLDollarQuote(sql string, i int) int {
	end := strings.IndexByte(sql[i+1:], '$')
	if end < 0 {
		return i
	}
	tag := sql[i : i+end+2]
	closeAt := strings.Index(sql[i+end+2:], tag)
	if closeAt < 0 {
		return i
	}
	return i + end + 2 + closeAt + len(tag)
}

func skipSQLLiteralOrComment(sql string, i int) (int, string, bool, bool) {
	if sql[i] == '\'' {
		for j := i + 1; j < len(sql); j++ {
			if sql[j] != '\'' {
				continue
			}
			if j+1 < len(sql) && sql[j+1] == '\'' {
				j++
				continue
			}
			return j + 1, "", false, false
		}
		return len(sql), "", false, false
	}
	if i+1 < len(sql) && sql[i+1] == '-' {
		if end := strings.IndexByte(sql[i+2:], '\n'); end >= 0 {
			return i + end + 2, "", false, false
		}
		return len(sql), "", false, false
	}
	return i, "", false, false
}
