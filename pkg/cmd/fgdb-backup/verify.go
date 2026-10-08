// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

package main

import (
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
)

// VerifyResult is the machine-readable verify summary.
type VerifyResult struct {
	OK       bool     `json:"ok"`
	Error    string   `json:"error,omitempty"`
	Backup   string   `json:"backup,omitempty"`
	Tables   int      `json:"tables,omitempty"`
	Rows     int64    `json:"rows,omitempty"`
	Files    int      `json:"files,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
}

// ListResult is the machine-readable list of complete backups.
type ListResult struct {
	OK         bool     `json:"ok"`
	Error      string   `json:"error,omitempty"`
	Latest     string   `json:"latest,omitempty"`
	Timestamps []string `json:"timestamps,omitempty"`
}

func runVerify(ctx context.Context, loc Location, src string) (VerifyResult, error) {
	var res VerifyResult
	root, rel, err := resolveBackup(ctx, loc, src)
	if err != nil {
		return res, err
	}
	store, base, err := openBackup(ctx, root, rel)
	if err != nil {
		return res, err
	}
	manifest, err := readManifest(ctx, store, base)
	if err != nil {
		return res, err
	}
	n, rows, err := verifyBackup(ctx, store, base, manifest)
	if err != nil {
		return res, err
	}
	res.OK = true
	res.Backup = root.String() + "/" + base
	res.Files = n
	res.Tables = len(manifest.Tables)
	res.Rows = rows
	res.Warnings = manifest.Warnings
	return res, nil
}

func verifyBackup(ctx context.Context, store Store, base string, manifest Manifest) (int, int64, error) {
	// Same order the backup writes metadata, then table files in the order
	// IMPORT reads them. objects.json is checked here before restore decodes it.
	var files []FileDigest
	files = append(files, manifest.SchemaFiles...)
	if manifest.UsersFile != nil {
		files = append(files, *manifest.UsersFile)
	}
	if manifest.ZonesFile != nil {
		files = append(files, *manifest.ZonesFile)
	}
	files = append(files, manifest.ObjectsFile)
	var rows int64
	for _, table := range manifest.Tables {
		var sum int64
		for _, f := range table.Files {
			files = append(files, f)
			sum += f.RowCount
		}
		if sum != table.RowCount {
			return 0, 0, fmt.Errorf("manifest row count for %s is %d but its files add up to %d", table.qualified(), table.RowCount, sum)
		}
		rows += table.RowCount
	}
	for _, f := range files {
		if err := verifyFile(ctx, store, base, manifest, f); err != nil {
			return 0, 0, err
		}
	}
	return len(files), rows, nil
}

type byteCounter struct {
	w io.Writer
	n int64
}

func (b *byteCounter) Write(p []byte) (int, error) {
	n, err := b.w.Write(p)
	b.n += int64(n)
	return n, err
}

func verifyFile(ctx context.Context, store Store, base string, manifest Manifest, f FileDigest) error {
	rc, err := store.Open(ctx, base+"/"+f.Path)
	if err != nil {
		return fmt.Errorf("%s: %w", f.Path, err)
	}
	defer rc.Close()
	h := sha256.New()
	counted := &byteCounter{w: h}
	src := io.TeeReader(rc, counted)
	if isDataFile(f.Path) {
		var payload io.Reader = src
		if manifest.Compression == "gzip" || hasGzipSuffix(f.Path) {
			zr, zerr := gzip.NewReader(src)
			if zerr != nil {
				return fmt.Errorf("%s: gzip: %w", f.Path, zerr)
			}
			defer zr.Close()
			payload = zr
		}
		n, cerr := countDataRows(payload, manifest.DataFormat)
		if cerr != nil {
			return fmt.Errorf("%s: %w", f.Path, cerr)
		}
		if n != f.RowCount {
			return fmt.Errorf("%s: counted %d rows, manifest says %d", f.Path, n, f.RowCount)
		}
	} else if _, err := io.Copy(io.Discard, src); err != nil {
		return fmt.Errorf("%s: %w", f.Path, err)
	}
	sum := hex.EncodeToString(h.Sum(nil))
	if sum != f.SHA256 {
		return fmt.Errorf("%s: sha256 is %s, manifest says %s", f.Path, sum, f.SHA256)
	}
	if counted.n != f.Bytes {
		return fmt.Errorf("%s: size is %d bytes, manifest says %d", f.Path, counted.n, f.Bytes)
	}
	return nil
}

func isDataFile(path string) bool {
	return len(path) >= 5 && (path == "data" || len(path) > 5 && path[:5] == "data/")
}

func hasGzipSuffix(path string) bool {
	return len(path) > 3 && path[len(path)-3:] == ".gz"
}

func runList(ctx context.Context, loc Location) (ListResult, error) {
	var res ListResult
	store, err := openStore(ctx, loc)
	if err != nil {
		return res, err
	}
	names, err := store.ListManifests(ctx, "")
	if err != nil {
		return res, err
	}
	res.Timestamps = names
	rc, err := store.Open(ctx, "latest.json")
	if err == nil {
		var ptr LatestPointer
		decErr := jsonNewDecoder(rc, &ptr)
		rc.Close()
		if decErr == nil && ptr.Complete {
			res.Latest = ptr.Timestamp
		}
	}
	res.OK = true
	return res, nil
}
