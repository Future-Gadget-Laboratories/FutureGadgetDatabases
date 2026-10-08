// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// Location is a local directory or an s3 prefix. Backup objects are paths
// relative to Root.
type Location struct {
	Kind       string // file or s3
	Root       string // local directory, or key prefix without leading slash
	Bucket     string
	Region     string
	Endpoint   string
	SSE        string
	KMSKeyID   string
	ImportAuth string // auto, implicit, specified
}

func parseLocation(raw, region, endpoint, sse, kms, importAuth string) (Location, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Location{}, fmt.Errorf("path is empty")
	}
	if importAuth == "" {
		importAuth = "auto"
	}
	switch importAuth {
	case "auto", "implicit", "specified":
	default:
		return Location{}, fmt.Errorf("--s3-import-auth must be auto, implicit, or specified")
	}
	switch sse {
	case "", "AES256", "aws:kms":
	default:
		return Location{}, fmt.Errorf("--sse must be AES256 or aws:kms")
	}
	if sse == "aws:kms" && kms == "" {
		return Location{}, fmt.Errorf("--sse aws:kms requires --sse-kms-key-id")
	}
	if strings.HasPrefix(raw, "s3://") {
		u, err := url.Parse(raw)
		if err != nil {
			return Location{}, err
		}
		if u.Host == "" {
			return Location{}, fmt.Errorf("s3 url %q is missing a bucket", raw)
		}
		if region == "" {
			region = os.Getenv("AWS_REGION")
		}
		if region == "" {
			region = os.Getenv("AWS_DEFAULT_REGION")
		}
		if region == "" {
			return Location{}, fmt.Errorf("s3 destinations need a region: set AWS_REGION or pass --s3-region")
		}
		root := strings.Trim(u.Path, "/")
		return Location{
			Kind:       "s3",
			Bucket:     u.Host,
			Root:       root,
			Region:     region,
			Endpoint:   endpoint,
			SSE:        sse,
			KMSKeyID:   kms,
			ImportAuth: importAuth,
		}, nil
	}
	abs, err := filepath.Abs(raw)
	if err != nil {
		return Location{}, err
	}
	return Location{Kind: "file", Root: abs, ImportAuth: importAuth}, nil
}

func (l Location) String() string {
	if l.Kind == "s3" {
		if l.Root == "" {
			return "s3://" + l.Bucket
		}
		return "s3://" + l.Bucket + "/" + l.Root
	}
	return l.Root
}

func (l Location) join(rel string) string {
	rel = path.Clean("/" + rel)[1:]
	if l.Kind == "s3" {
		if l.Root == "" {
			return rel
		}
		if rel == "" {
			return l.Root
		}
		return l.Root + "/" + rel
	}
	return filepath.Join(l.Root, filepath.FromSlash(rel))
}

// Store reads and writes a backup directory. Writes land under a temporary
// name until Close, so a crash does not leave a file that looks complete.
type Store interface {
	Create(ctx context.Context, rel string) (io.WriteCloser, error)
	Open(ctx context.Context, rel string) (io.ReadCloser, error)
	Exists(ctx context.Context, rel string) (bool, error)
	// ListDirs returns child directory names of rel that contain manifest.json
	// when rel is a name prefix, or the names of timestamp directories.
	ListManifests(ctx context.Context, rel string) ([]string, error)
}

func openStore(ctx context.Context, loc Location) (Store, error) {
	switch loc.Kind {
	case "file":
		if err := os.MkdirAll(loc.Root, 0o755); err != nil {
			return nil, err
		}
		return &localStore{root: loc.Root}, nil
	case "s3":
		return newS3Store(ctx, loc)
	default:
		return nil, fmt.Errorf("unknown location %q", loc.Kind)
	}
}

type localStore struct {
	root string
}

func (s *localStore) path(rel string) string {
	rel = path.Clean("/" + rel)[1:]
	return filepath.Join(s.root, filepath.FromSlash(rel))
}

func (s *localStore) Create(_ context.Context, rel string) (io.WriteCloser, error) {
	final := s.path(rel)
	if err := os.MkdirAll(filepath.Dir(final), 0o755); err != nil {
		return nil, err
	}
	tmp := final + ".partial"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	return &renameFile{f: f, tmp: tmp, final: final}, nil
}

func (s *localStore) Open(_ context.Context, rel string) (io.ReadCloser, error) {
	return os.Open(s.path(rel))
}

func (s *localStore) Exists(_ context.Context, rel string) (bool, error) {
	_, err := os.Stat(s.path(rel))
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func (s *localStore) ListManifests(_ context.Context, rel string) ([]string, error) {
	dir := s.root
	if rel != "" {
		dir = s.path(rel)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		manifest := filepath.Join(dir, e.Name(), "manifest.json")
		if _, err := os.Stat(manifest); err == nil {
			out = append(out, e.Name())
		}
	}
	return out, nil
}

type renameFile struct {
	f     *os.File
	tmp   string
	final string
	done  bool
}

func (r *renameFile) Write(p []byte) (int, error) { return r.f.Write(p) }

func (r *renameFile) Close() error {
	if r.done {
		return nil
	}
	r.done = true
	if err := r.f.Sync(); err != nil {
		_ = r.f.Close()
		_ = os.Remove(r.tmp)
		return err
	}
	if err := r.f.Close(); err != nil {
		_ = os.Remove(r.tmp)
		return err
	}
	if err := os.Rename(r.tmp, r.final); err != nil {
		_ = os.Remove(r.tmp)
		return err
	}
	return nil
}

// partWriter keeps at most partSize bytes buffered and flushes full parts
// immediately. The last part may be shorter.
type partWriter struct {
	partSize int
	buf      []byte
	flush    func(part []byte) error
	maxBuf   int
	parts    int
}

func newPartWriter(partSize int, flush func(part []byte) error) *partWriter {
	if partSize < 1 {
		partSize = 8 << 20
	}
	return &partWriter{
		partSize: partSize,
		buf:      make([]byte, 0, partSize),
		flush:    flush,
	}
}

func (w *partWriter) Write(p []byte) (int, error) {
	written := 0
	for len(p) > 0 {
		space := w.partSize - len(w.buf)
		n := len(p)
		if n > space {
			n = space
		}
		w.buf = append(w.buf, p[:n]...)
		p = p[n:]
		written += n
		if len(w.buf) > w.maxBuf {
			w.maxBuf = len(w.buf)
		}
		if len(w.buf) == w.partSize {
			if err := w.emit(); err != nil {
				return written, err
			}
		}
	}
	return written, nil
}

func (w *partWriter) emit() error {
	if len(w.buf) == 0 {
		return nil
	}
	part := make([]byte, len(w.buf))
	copy(part, w.buf)
	w.buf = w.buf[:0]
	w.parts++
	return w.flush(part)
}

func (w *partWriter) Close() error { return w.emit() }

type hashWriteCloser struct {
	w io.WriteCloser
	h hash.Hash
	n int64
}

func newHashWriteCloser(w io.WriteCloser) *hashWriteCloser {
	return &hashWriteCloser{w: w, h: sha256.New()}
}

func (h *hashWriteCloser) Write(p []byte) (int, error) {
	n, err := h.w.Write(p)
	if n > 0 {
		_, _ = h.h.Write(p[:n])
		h.n += int64(n)
	}
	return n, err
}

func (h *hashWriteCloser) Close() error { return h.w.Close() }

func (h *hashWriteCloser) digest() FileDigest {
	return FileDigest{
		SHA256: hex.EncodeToString(h.h.Sum(nil)),
		Bytes:  h.n,
	}
}

type deadlineWriter struct {
	w        io.Writer
	exceeded func() error
}

func (d deadlineWriter) Write(p []byte) (int, error) {
	if d.exceeded != nil {
		if err := d.exceeded(); err != nil {
			return 0, err
		}
	}
	return d.w.Write(p)
}
