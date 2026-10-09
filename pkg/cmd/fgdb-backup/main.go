// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

// Command fgdb-backup copies user databases to and from a directory or S3
// using SQL that the open-source binary accepts.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "backup":
		os.Exit(cmdBackup(os.Args[2:]))
	case "restore":
		os.Exit(cmdRestore(os.Args[2:]))
	case "verify":
		os.Exit(cmdVerify(os.Args[2:]))
	case "list":
		os.Exit(cmdList(os.Args[2:]))
	case "unlock":
		os.Exit(cmdUnlock(os.Args[2:]))
	case "help", "-h", "--help":
		usage()
		os.Exit(0)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `fgdb-backup copies CockroachDB user databases with SQL.

  fgdb-backup backup  --url URL --dest s3://bucket/prefix [--database name]
  fgdb-backup restore --url URL --src  s3://bucket/prefix/name/latest
  fgdb-backup verify  --src  s3://bucket/prefix/name/latest
  fgdb-backup list    --src  s3://bucket/prefix/name
  fgdb-backup unlock  --dest s3://bucket/prefix --name name --yes

--url is a PostgreSQL connection URL. Put certificates in the URL:
  postgresql://root@host:26257/defaultdb?sslmode=verify-full&sslrootcert=ca.crt&sslcert=client.root.crt&sslkey=client.root.key

Exit status is 0 on success, 1 on a failed backup or restore, and 2 on a bad command.
Use --json to print one JSON object on stdout. Progress lines go to stderr.
`)
}

const (
	flagS3Region     = "s3-region"
	flagS3Endpoint   = "s3-endpoint"
	flagExtendGCTTL  = "extend-gc-ttl"
	flagSplitRows    = "split-rows"
	flagPartSize     = "part-size"
	flagSafetyMargin = "safety-margin"
	flagJSON         = "json"

	helpS3Region   = "S3 region. Default: AWS_REGION"
	helpS3Endpoint = "S3-compatible endpoint"
	helpJSON       = "print a JSON object on stdout"
)

type stringList []string

func (s *stringList) String() string { return strings.Join(*s, ",") }

func (s *stringList) Set(v string) error {
	for _, p := range strings.Split(v, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			*s = append(*s, p)
		}
	}
	return nil
}

func cmdBackup(args []string) int {
	cfgPath := configPath(args)
	cfg, err := readBackupConfig(cfgPath)
	if err != nil {
		return fail(false, err)
	}
	if err := validateConfig(cfg); err != nil {
		return fail(false, err)
	}
	fs := flag.NewFlagSet("backup", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.String("config", cfgPath, "YAML configuration file")
	urlStr := fs.String("url", "", "PostgreSQL connection URL")
	dest := fs.String("dest", "", "s3://bucket/prefix or a local directory")
	var dbs stringList
	fs.Var(&dbs, "database", "database to include; repeat the flag or use commas. Default: all user databases")
	name := fs.String("name", "", "folder name under --dest. Default: the cluster id")
	compression := fs.String("compression", "gzip", "gzip or none")
	sse := fs.String("sse", "", "S3 server-side encryption: AES256 or aws:kms")
	kms := fs.String("sse-kms-key-id", "", "KMS key id or ARN when --sse=aws:kms")
	region := fs.String(flagS3Region, "", helpS3Region)
	endpoint := fs.String(flagS3Endpoint, "", "S3-compatible endpoint, for example http://127.0.0.1:9000")
	extend := fs.String(flagExtendGCTTL, "", "temporarily raise gc.ttlseconds for this run, for example 12h")
	split := fs.Int(flagSplitRows, 0, "split integer-primary-key tables into ranges of this many rows")
	part := fs.Int(flagPartSize, 8<<20, "S3 multipart part size in bytes (minimum 5242880)")
	margin := fs.Duration(flagSafetyMargin, time.Minute, "fail before the snapshot's GC deadline gets this close")
	asJSON := fs.Bool(flagJSON, false, helpJSON)
	threads := fs.Int("threads", configInt(cfg.Backup.Threads, 1), "maximum Go processor threads")
	memoryBytes := fs.Int64("memory-bytes", configInt64(cfg.Backup.MemoryBytes, 1<<30), "soft memory limit in bytes")
	lockDefault := configBool(cfg.Backup.Lock, true)
	lock := fs.String("lock", map[bool]string{true: "on", false: "off"}[lockDefault], "same-name backup lock: on or off")
	lockWaitDefault, err := configuredDuration(cfg.Backup.LockWait, "backup.lock_wait", 0)
	if err != nil {
		return fail(*asJSON, err)
	}
	lockWait := fs.Duration("lock-wait", lockWaitDefault, "wait for a same-name backup lock")
	lockLeaseDefault, err := configuredDuration(cfg.Backup.LockLease, "backup.lock_lease", 10*time.Minute)
	if err != nil {
		return fail(*asJSON, err)
	}
	lockLease := fs.Duration("lock-lease", lockLeaseDefault, "lease duration for a same-name backup lock")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *urlStr == "" || *dest == "" || fs.NArg() != 0 {
		usage()
		return 2
	}
	applyResourceCaps(*threads, *memoryBytes)
	var extendDur time.Duration
	if *extend != "" {
		var err error
		extendDur, err = time.ParseDuration(*extend)
		if err != nil {
			return fail(*asJSON, fmt.Errorf("--extend-gc-ttl: %w", err))
		}
	}
	if *lock != "on" && *lock != "off" {
		return fail(*asJSON, fmt.Errorf("--lock must be on or off"))
	}
	loc, err := parseLocation(*dest, *region, *endpoint, *sse, *kms, cfg.Backup.S3CredentialMode)
	if err != nil {
		return fail(*asJSON, err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	res, err := runBackup(ctx, BackupOptions{
		URL:          *urlStr,
		Dest:         loc,
		Databases:    dbs,
		Name:         *name,
		Compression:  *compression,
		SplitRows:    *split,
		PartSize:     *part,
		SafetyMargin: *margin,
		ExtendGCTTL:  extendDur,
		JSON:         *asJSON,
		Lock:         *lock == "on",
		LockWait:     *lockWait,
		LockLease:    *lockLease,
		ConfigPath:   cfgPath,
	})
	if err != nil {
		res.OK = false
		if errors.Is(err, context.Canceled) && !strings.Contains(err.Error(), "could not restore gc.ttlseconds") {
			err = fmt.Errorf("backup interrupted by signal; gc.ttlseconds was restored when this run had raised it")
		}
		if res.Error == "" {
			res.Error = err.Error()
		}
		emit(*asJSON, res, false)
		fmt.Fprintf(os.Stderr, "backup failed: %s\n", err)
		return 1
	}
	emit(*asJSON, res, true)
	return 0
}

func cmdRestore(args []string) int {
	cfgPath := configPath(args)
	cfg, err := readBackupConfig(cfgPath)
	if err != nil {
		return fail(false, err)
	}
	if err := validateConfig(cfg); err != nil {
		return fail(false, err)
	}
	fs := flag.NewFlagSet("restore", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.String("config", cfgPath, "YAML configuration file")
	urlStr := fs.String("url", "", "PostgreSQL connection URL")
	src := fs.String("src", "", "backup timestamp directory, or a path ending in /latest")
	var dbs stringList
	fs.Var(&dbs, "database", "database to restore; repeat the flag or use commas. Default: every database in the backup")
	force := fs.Bool("force", false, "drop and recreate only the objects in the backup when the target database is not empty")
	load := fs.String("load", "import", "import (IMPORT INTO) or copy (COPY FROM STDIN)")
	listen := fs.String("import-listen", "127.0.0.1:0", "address the database dials when importing a local backup")
	region := fs.String(flagS3Region, "", helpS3Region)
	endpoint := fs.String(flagS3Endpoint, "", helpS3Endpoint)
	importAuth := fs.String("s3-import-auth", "auto", "how the database reads S3: auto, implicit, or specified")
	swap := fs.Bool("swap-restore", configBool(cfg.Restore.SwapRestore, true), "restore beside an existing database and swap names when possible")
	plan := fs.Bool("plan", false, "show the restore preflight without changing the cluster")
	planFormat := fs.String("plan-format", "text", "plan output: text or json")
	testingMode := fs.Bool("testing-mode", configBool(cfg.Restore.TestingMode, false), "allow test-only S3 probes")
	threads := fs.Int("threads", configInt(cfg.Backup.Threads, 1), "maximum Go processor threads")
	memoryBytes := fs.Int64("memory-bytes", configInt64(cfg.Backup.MemoryBytes, 1<<30), "soft memory limit in bytes")
	asJSON := fs.Bool(flagJSON, false, helpJSON)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *urlStr == "" || *src == "" || fs.NArg() != 0 {
		usage()
		return 2
	}
	applyResourceCaps(*threads, *memoryBytes)
	if *asJSON && *plan {
		*planFormat = "json"
	}
	if *planFormat != "text" && *planFormat != "json" {
		return fail(*asJSON, fmt.Errorf("--plan-format must be text or json"))
	}
	if *testingMode {
		// The mode is carried in RestoreOptions. It is intentionally not
		// inferred from a development endpoint or environment variable.
	}
	if cfg.Restore.S3CredentialMode != "" && *importAuth == "auto" {
		*importAuth = cfg.Restore.S3CredentialMode
	}
	loc, err := parseLocation(*src, *region, *endpoint, "", "", *importAuth)
	if err != nil {
		return fail(*asJSON, err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	res, err := runRestore(ctx, RestoreOptions{
		URL:          *urlStr,
		Src:          loc.String(),
		Location:     loc,
		Databases:    dbs,
		Force:        *force,
		Load:         *load,
		ImportListen: *listen,
		JSON:         *asJSON,
		Plan:         *plan,
		PlanFormat:   *planFormat,
		SwapRestore:  *swap,
		TestingMode:  *testingMode,
		ConfigPath:   cfgPath,
	})
	if err != nil {
		res.OK = false
		if res.Error == "" {
			res.Error = err.Error()
		}
		emit(*asJSON, res, false)
		fmt.Fprintf(os.Stderr, "restore failed: %s\n", err)
		var planErr *planRefusalError
		if errors.As(err, &planErr) {
			return 4
		}
		return 1
	}
	emit(*asJSON, res, true)
	return 0
}

func cmdVerify(args []string) int {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	src := fs.String("src", "", "backup timestamp directory, or a path ending in /latest")
	region := fs.String(flagS3Region, "", helpS3Region)
	endpoint := fs.String(flagS3Endpoint, "", helpS3Endpoint)
	asJSON := fs.Bool(flagJSON, false, helpJSON)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *src == "" || fs.NArg() != 0 {
		usage()
		return 2
	}
	loc, err := parseLocation(*src, *region, *endpoint, "", "", "auto")
	if err != nil {
		return fail(*asJSON, err)
	}
	res, err := runVerify(context.Background(), loc, loc.String())
	if err != nil {
		res.OK = false
		if res.Error == "" {
			res.Error = err.Error()
		}
		emit(*asJSON, res, false)
		fmt.Fprintf(os.Stderr, "verify failed: %s\n", err)
		return 1
	}
	emit(*asJSON, res, true)
	return 0
}

func cmdList(args []string) int {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	src := fs.String("src", "", "directory that contains timestamp folders, for example s3://bucket/prefix/name")
	region := fs.String(flagS3Region, "", helpS3Region)
	endpoint := fs.String(flagS3Endpoint, "", helpS3Endpoint)
	asJSON := fs.Bool(flagJSON, false, helpJSON)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *src == "" || fs.NArg() != 0 {
		usage()
		return 2
	}
	loc, err := parseLocation(*src, *region, *endpoint, "", "", "auto")
	if err != nil {
		return fail(*asJSON, err)
	}
	res, err := runList(context.Background(), loc)
	if err != nil {
		res.OK = false
		if res.Error == "" {
			res.Error = err.Error()
		}
		emit(*asJSON, res, false)
		fmt.Fprintf(os.Stderr, "list failed: %s\n", err)
		return 1
	}
	emit(*asJSON, res, true)
	return 0
}

func cmdUnlock(args []string) int {
	fs := flag.NewFlagSet("unlock", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	dest := fs.String("dest", "", "backup destination")
	name := fs.String("name", "", "backup name")
	region := fs.String(flagS3Region, "", helpS3Region)
	endpoint := fs.String(flagS3Endpoint, "", helpS3Endpoint)
	yes := fs.Bool("yes", false, "confirm removal of the lock")
	force := fs.Bool("force-unlock", false, "remove a live lease too")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *dest == "" || *name == "" || fs.NArg() != 0 {
		return 2
	}
	loc, err := parseLocation(*dest, *region, *endpoint, "", "", "auto")
	if err != nil {
		return fail(false, err)
	}
	if !*yes {
		return fail(false, errors.New("unlock requires --yes"))
	}
	if err := unlockBackup(context.Background(), loc, *name, *force); err != nil {
		return fail(false, err)
	}
	fmt.Printf("unlocked %s\n", *name)
	return 0
}

func fail(asJSON bool, err error) int {
	emit(asJSON, map[string]any{"ok": false, "error": err.Error()}, false)
	fmt.Fprintf(os.Stderr, "%s\n", err)
	return 2
}

func emit(asJSON bool, v any, human bool) {
	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(v)
		return
	}
	if !human {
		return
	}
	switch t := v.(type) {
	case BackupResult:
		fmt.Printf("backup complete\n  dest: %s\n  latest: %s\n  as_of: %s\n  tables: %d\n  rows: %d\n  peak_rss_bytes: %d\n",
			t.Backup, t.Latest, t.AsOf, t.Tables, t.Rows, t.PeakRSSBytes)
		for _, w := range t.Warnings {
			fmt.Printf("  warning: %s\n", w)
		}
	case RestoreResult:
		fmt.Printf("restore complete\n  backup: %s\n  tables: %d\n  rows: %d\n", t.Backup, t.Tables, t.Rows)
		for _, w := range t.Warnings {
			fmt.Printf("  warning: %s\n", w)
		}
	case VerifyResult:
		fmt.Printf("verify complete\n  backup: %s\n  files: %d\n  tables: %d\n  rows: %d\n", t.Backup, t.Files, t.Tables, t.Rows)
	case ListResult:
		fmt.Printf("latest: %s\n", t.Latest)
		for _, ts := range t.Timestamps {
			fmt.Printf("%s\n", ts)
		}
	}
}
