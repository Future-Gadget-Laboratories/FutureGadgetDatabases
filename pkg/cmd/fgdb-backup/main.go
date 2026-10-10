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
	case "prune":
		os.Exit(cmdPrune(os.Args[2:]))
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
  fgdb-backup prune   --url URL --name name [--keep 1] [--dry-run]

--url is a PostgreSQL connection URL. Put certificates in the URL:
  postgresql://root@host:26257/defaultdb?sslmode=verify-full&sslrootcert=ca.crt&sslcert=client.root.crt&sslkey=client.root.key

Exit status:
  0  the command finished
  1  the command started, then failed
  2  the command line is wrong
  4  restore --plan refused the restore. A real restore that is refused exits 1.

Use --json to print one JSON object on stdout. Progress lines go to stderr.
`)
}

const (
	flagS3Region             = "s3-region"
	flagS3Endpoint           = "s3-endpoint"
	flagAllowUnsafeOverwrite = "allow-unsafe-overwrite"
	flagExtendGCTTL          = "extend-gc-ttl"
	flagSplitRows            = "split-rows"
	flagPartSize             = "part-size"
	flagSafetyMargin         = "safety-margin"
	flagJSON                 = "json"
	flagServeAddr            = "serve-addr"
	flagImportListen         = "import-listen"
	flagServeAdvertise       = "serve-advertise"
	flagServeTLSCert         = "serve-tls-cert"
	flagServeTLSKey          = "serve-tls-key"

	defaultImportListen = "127.0.0.1:0"

	helpS3Region             = "S3 region. Default: AWS_REGION"
	helpS3Endpoint           = "S3-compatible endpoint"
	helpJSON                 = "print a JSON object on stdout"
	helpPostgresURL          = "PostgreSQL connection URL"
	helpAllowUnsafeOverwrite = "allow S3 destinations that ignore If-None-Match"
	helpTestingMode          = "create a temporary probe table, copy one row into it, and drop it"
	helpS3ImportAuth         = "S3 credential mode: " + s3ImportAuthList
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

type forceModeFlag struct {
	set   bool
	value string
}

func (f *forceModeFlag) String() string { return f.value }

func (f *forceModeFlag) Set(value string) error {
	switch value {
	case "true", "swap":
		f.value = "swap"
	case "in-place":
		f.value = value
	default:
		return fmt.Errorf("--force must be used alone or set to in-place")
	}
	f.set = true
	return nil
}

func (f *forceModeFlag) IsBoolFlag() bool { return true }

func cmdBackup(args []string) int {
	cfgPath, configErr := configPath(args)
	if configErr != nil {
		return fail(false, configErr)
	}
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
	urlStr := fs.String("url", "", helpPostgresURL)
	dest := fs.String("dest", "", "s3://bucket/prefix or a local directory")
	var dbs stringList
	fs.Var(&dbs, "database", "database to include; repeat the flag or use commas. Default: all user databases")
	name := fs.String("name", "", "folder name under --dest. Default: the cluster id")
	compression := fs.String("compression", "gzip", "gzip or none")
	sse := fs.String("sse", "", "S3 server-side encryption: AES256 or aws:kms")
	kms := fs.String("sse-kms-key-id", "", "KMS key id or ARN when --sse=aws:kms")
	region := fs.String(flagS3Region, "", helpS3Region)
	endpoint := fs.String(flagS3Endpoint, "", "S3-compatible endpoint, for example http://127.0.0.1:9000")
	s3AuthDefault := cfg.Backup.S3CredentialMode
	if s3AuthDefault == "" {
		s3AuthDefault = "auto"
	}
	s3Auth := fs.String("s3-import-auth", s3AuthDefault, helpS3ImportAuth)
	allowUnsafe := fs.Bool(flagAllowUnsafeOverwrite, configBool(cfg.Backup.AllowUnsafeOverwrite, false), helpAllowUnsafeOverwrite)
	extend := fs.String(flagExtendGCTTL, "", "temporarily raise gc.ttlseconds for this run, for example 12h")
	split := fs.Int(flagSplitRows, 0, "split integer-primary-key tables into ranges of this many rows")
	part := fs.Int(flagPartSize, 8<<20, "S3 multipart part size in bytes (minimum 5242880)")
	margin := fs.Duration(flagSafetyMargin, time.Minute, "fail before the snapshot's GC deadline gets this close")
	skipGrants := fs.Bool("skip-grants", false, "skip grants if the source does not allow reading them; record the skip in the backup manifest")
	asJSON := fs.Bool(flagJSON, false, helpJSON)
	threads := fs.Int("threads", configInt(cfg.Backup.Threads, 0), "maximum Go processor threads; 0 means no cap")
	memoryBytes := fs.Int64("memory-bytes", configInt64(cfg.Backup.MemoryBytes, 0), "soft memory limit in bytes; 0 means no cap")
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
	return runParsedBackup(parsedBackup{
		url: *urlStr, dest: *dest, name: *name, compression: *compression,
		sse: *sse, kms: *kms, region: *region, endpoint: *endpoint, s3Auth: *s3Auth,
		extend: *extend, lock: *lock, databases: dbs, allowUnsafe: *allowUnsafe,
		skipGrants: *skipGrants, asJSON: *asJSON, split: *split, part: *part,
		margin: *margin, lockWait: *lockWait, lockLease: *lockLease,
		threads: *threads, memoryBytes: *memoryBytes, fileMode: cfg.Backup.FileMode,
		cfgPath: cfgPath, extra: fs.NArg(),
	})
}

type parsedBackup struct {
	url, dest, name, compression    string
	sse, kms, region, endpoint      string
	s3Auth, extend, lock, fileMode  string
	cfgPath                         string
	databases                       []string
	allowUnsafe, skipGrants, asJSON bool
	split, part, threads, extra     int
	memoryBytes                     int64
	margin, lockWait, lockLease     time.Duration
}

func runParsedBackup(flag parsedBackup) int {
	if flag.url == "" || flag.dest == "" || flag.extra != 0 {
		usage()
		return 2
	}
	applyResourceCaps(flag.threads, flag.memoryBytes)
	extendDur, err := backupExtend(flag.extend)
	if err != nil {
		return fail(flag.asJSON, err)
	}
	if err := requireLockMode(flag.lock); err != nil {
		return fail(flag.asJSON, err)
	}
	loc, err := backupWriteLocation(flag)
	if err != nil {
		return fail(flag.asJSON, err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	return emitBackup(flag, ctx, loc, extendDur)
}

func backupExtend(raw string) (time.Duration, error) {
	if raw == "" {
		return 0, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("--extend-gc-ttl: %w", err)
	}
	return d, nil
}

func requireLockMode(lock string) error {
	if lock != "on" && lock != "off" {
		return fmt.Errorf("--lock must be on or off")
	}
	return nil
}

func backupWriteLocation(flag parsedBackup) (Location, error) {
	loc, err := parseLocation(flag.dest, flag.region, flag.endpoint, flag.sse, flag.kms, flag.s3Auth)
	if err != nil {
		return Location{}, err
	}
	loc.FileMode, err = configuredFileMode(flag.fileMode)
	if err != nil {
		return Location{}, err
	}
	loc.AllowUnsafeOverwrite = flag.allowUnsafe
	loc.ProbeConditionalWrites = true
	return loc, nil
}

func emitBackup(flag parsedBackup, ctx context.Context, loc Location, extendDur time.Duration) int {
	res, err := runBackup(ctx, BackupOptions{
		URL: flag.url, Dest: loc, Databases: flag.databases, Name: flag.name,
		Compression: flag.compression, SplitRows: flag.split, PartSize: flag.part,
		SafetyMargin: flag.margin, ExtendGCTTL: extendDur, SkipGrants: flag.skipGrants,
		JSON: flag.asJSON, Lock: flag.lock == "on", LockWait: flag.lockWait,
		LockLease: flag.lockLease, ConfigPath: flag.cfgPath,
	})
	if err != nil {
		return failBackup(flag.asJSON, res, err)
	}
	emit(flag.asJSON, res, true)
	return 0
}

func failBackup(asJSON bool, res BackupResult, err error) int {
	res.OK = false
	if interruptedBackup(err) {
		err = fmt.Errorf("backup interrupted by signal; gc.ttlseconds was restored when this run had raised it")
	}
	if res.Error == "" {
		res.Error = err.Error()
	}
	emit(asJSON, res, false)
	fmt.Fprintf(os.Stderr, "backup failed: %s\n", err)
	return 1
}

func interruptedBackup(err error) bool {
	if !errors.Is(err, context.Canceled) {
		return false
	}
	return !strings.Contains(err.Error(), "could not restore gc.ttlseconds")
}

func cmdRestore(args []string) int {
	cfgPath, configErr := configPath(args)
	if configErr != nil {
		return fail(false, configErr)
	}
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
	urlStr := fs.String("url", "", helpPostgresURL)
	src := fs.String("src", "", "backup timestamp directory, or a path ending in /latest")
	var dbs stringList
	fs.Var(&dbs, "database", "database to restore; repeat the flag or use commas. Default: every database in the backup")
	var force forceModeFlag
	fs.Var(&force, "force", "force a non-empty restore; swap by default, or use --force=in-place as a last resort")
	load := fs.String("load", "import", "import (IMPORT INTO) or copy (COPY FROM STDIN)")
	listen := fs.String(flagImportListen, defaultImportListen, "address the database dials when importing a local backup")
	serveAddr := fs.String(flagServeAddr, "", "address the local import file server binds; used instead of --"+flagImportListen+" when set")
	serveAdvertise := fs.String(flagServeAdvertise, "", "host or host:port the database nodes dial; required for an unspecified bind such as :8080, 0:8080, 0.0.0.0, or [::]")
	serveTLSCert := fs.String(flagServeTLSCert, "", "TLS certificate for the local import file server")
	serveTLSKey := fs.String(flagServeTLSKey, "", "TLS private key for the local import file server")
	region := fs.String(flagS3Region, "", helpS3Region)
	endpoint := fs.String(flagS3Endpoint, "", helpS3Endpoint)
	importAuthDefault := cfg.Restore.S3CredentialMode
	if importAuthDefault == "" {
		importAuthDefault = "auto"
	}
	importAuth := fs.String("s3-import-auth", importAuthDefault, helpS3ImportAuth)
	allowUnsafe := fs.Bool(flagAllowUnsafeOverwrite, configBool(cfg.Restore.AllowUnsafeOverwrite, false), helpAllowUnsafeOverwrite)
	swap := fs.Bool("swap-restore", configBool(cfg.Restore.SwapRestore, true), "restore beside an existing database and swap names when possible")
	retention := fs.Int("retention", configInt(cfg.Restore.Retention, 1), "number of old swapped database copies to keep")
	plan := fs.Bool("plan", false, "show the restore preflight without changing the cluster")
	planFormat := fs.String("plan-format", "text", "plan output: text or json")
	testingMode := fs.Bool("testing-mode", configBool(cfg.Restore.TestingMode, false), helpTestingMode)
	threads := fs.Int("threads", configInt(cfg.Restore.Threads, 0), "maximum Go processor threads; 0 means no cap")
	memoryBytes := fs.Int64("memory-bytes", configInt64(cfg.Restore.MemoryBytes, 0), "soft memory limit in bytes; 0 means no cap")
	asJSON := fs.Bool(flagJSON, false, helpJSON)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	format, err := planFormatValue(*asJSON, *plan, *planFormat)
	if err != nil {
		return fail(*asJSON, err)
	}
	return runParsedRestore(parsedRestore{
		url: *urlStr, src: *src, databases: dbs, force: force, load: *load,
		listen: *listen, serveAddr: *serveAddr, serveAdvertise: *serveAdvertise,
		serveTLSCert: *serveTLSCert, serveTLSKey: *serveTLSKey,
		region: *region, endpoint: *endpoint, importAuth: *importAuth,
		allowUnsafe: *allowUnsafe, swap: *swap, plan: *plan, testingMode: *testingMode,
		asJSON: *asJSON, retention: *retention, planFormat: format,
		threads: *threads, memoryBytes: *memoryBytes,
		yamlInPlace: configBool(cfg.Restore.InPlace, false),
		cfgPath:     cfgPath, extra: fs.NArg(),
	})
}

type parsedRestore struct {
	url, src, load, listen, serveAddr, serveAdvertise string
	serveTLSCert, serveTLSKey, region, endpoint       string
	importAuth, planFormat, cfgPath                   string
	databases                                         []string
	force                                             forceModeFlag
	allowUnsafe, swap, plan, testingMode, asJSON      bool
	yamlInPlace                                       bool
	retention, threads, extra                         int
	memoryBytes                                       int64
}

func planFormatValue(asJSON, plan bool, format string) (string, error) {
	if asJSON && plan {
		format = "json"
	}
	if format != "text" && format != "json" {
		return "", fmt.Errorf("--plan-format must be text or json")
	}
	return format, nil
}

func runParsedRestore(flag parsedRestore) int {
	if flag.url == "" || flag.src == "" || flag.extra != 0 {
		usage()
		return 2
	}
	applyResourceCaps(flag.threads, flag.memoryBytes)
	loc, err := parseLocation(flag.src, flag.region, flag.endpoint, "", "", flag.importAuth)
	if err != nil {
		return fail(flag.asJSON, err)
	}
	loc.AllowUnsafeOverwrite = flag.allowUnsafe
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	res, err := runRestore(ctx, restoreOptionsFromFlags(flag, loc))
	if err != nil {
		return failRestore(flag.asJSON, res, err)
	}
	emit(flag.asJSON, res, true)
	return 0
}

func restoreOptionsFromFlags(flag parsedRestore, loc Location) RestoreOptions {
	return RestoreOptions{
		URL: flag.url, Src: loc.String(), Location: loc, Databases: flag.databases,
		Force: flag.force.set, InPlace: restoreInPlace(flag.force, flag.yamlInPlace),
		Load: flag.load, ImportListen: flag.listen, ServeAddr: flag.serveAddr,
		ServeAdvertise: flag.serveAdvertise, ServeTLSCert: flag.serveTLSCert,
		ServeTLSKey: flag.serveTLSKey, JSON: flag.asJSON, Plan: flag.plan,
		PlanFormat: flag.planFormat, SwapRestore: flag.swap, Retention: flag.retention,
		TestingMode: flag.testingMode, ConfigPath: flag.cfgPath,
	}
}

func restoreInPlace(force forceModeFlag, yamlInPlace bool) bool {
	if force.value == "in-place" {
		return true
	}
	if force.set {
		return false
	}
	return yamlInPlace
}

func failRestore(asJSON bool, res RestoreResult, err error) int {
	res.OK = false
	if res.Error == "" {
		res.Error = err.Error()
	}
	emit(asJSON, res, false)
	fmt.Fprintf(os.Stderr, "restore failed: %s\n", err)
	return restoreStatus(err)
}

func cmdVerify(args []string) int {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	src := fs.String("src", "", "backup timestamp directory, or a path ending in /latest")
	region := fs.String(flagS3Region, "", helpS3Region)
	endpoint := fs.String(flagS3Endpoint, "", helpS3Endpoint)
	allowUnsafe := fs.Bool(flagAllowUnsafeOverwrite, false, helpAllowUnsafeOverwrite)
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
	loc.AllowUnsafeOverwrite = *allowUnsafe
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
	if fs.NArg() != 0 {
		usage()
		return 2
	}
	if *dest == "" || *name == "" {
		fmt.Fprintf(os.Stderr, "unlock requires --dest and --name\n")
		return 2
	}
	loc, err := parseLocation(*dest, *region, *endpoint, "", "", "auto")
	if err != nil {
		return fail(false, err)
	}
	loc.ProbeConditionalWrites = true
	if !*yes {
		return fail(false, errors.New("unlock requires --yes"))
	}
	if err := unlockBackup(context.Background(), loc, *name, *force); err != nil {
		return fail(false, err)
	}
	fmt.Printf("unlocked %s\n", *name)
	return 0
}

func cmdPrune(args []string) int {
	fs := flag.NewFlagSet("prune", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	urlStr := fs.String("url", "", helpPostgresURL)
	name := fs.String("name", "", "original database name")
	keep := fs.Int("keep", 1, "number of old swapped copies to keep")
	dryRun := fs.Bool("dry-run", false, "show old copies that would be dropped, without dropping them")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *urlStr == "" || *name == "" || fs.NArg() != 0 {
		usage()
		return 2
	}
	if err := safeSegment(*name); err != nil {
		return fail(false, err)
	}
	db, err := connect(context.Background(), *urlStr)
	if err != nil {
		return fail(false, err)
	}
	defer db.Close(context.Background())
	removed, err := pruneOldCopies(context.Background(), db, *name, *keep, *dryRun)
	if err != nil {
		return fail(false, err)
	}
	reportPrune(removed, *dryRun)
	return 0
}

func reportPrune(removed []string, dryRun bool) {
	if dryRun {
		for _, database := range removed {
			fmt.Printf("would drop %s\n", database)
		}
		fmt.Printf("dry-run: %d old database copies\n", len(removed))
		return
	}
	fmt.Printf("pruned %d old database copies\n", len(removed))
}

func restoreStatus(err error) int {
	var planErr *planRefusalError
	if errors.As(err, &planErr) {
		return 4
	}
	return 1
}

func fail(asJSON bool, err error) int {
	emit(asJSON, map[string]any{"ok": false, "error": err.Error()}, false)
	fmt.Fprintf(os.Stderr, "%s\n", err)
	return 1
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
