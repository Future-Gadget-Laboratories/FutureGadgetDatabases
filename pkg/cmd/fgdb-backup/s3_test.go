// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

package main

import (
	"bytes"
	"context"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/johannesboyne/gofakes3"
	"github.com/johannesboyne/gofakes3/backend/s3mem"
)

func TestS3Multipart(t *testing.T) {
	backend := s3mem.New()
	if err := backend.CreateBucket("lab"); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(gofakes3.New(backend).Server())
	defer srv.Close()
	t.Setenv("AWS_ACCESS_KEY_ID", "testkey")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "testsecret")
	t.Setenv("AWS_REGION", "us-east-1")

	loc, err := parseLocation("s3://lab/fgdb", "us-east-1", srv.URL, "", "", "specified")
	if err != nil {
		t.Fatal(err)
	}
	store, err := openStore(context.Background(), loc)
	if err != nil {
		t.Fatal(err)
	}
	s3s := store.(*s3Store)
	s3s.partSize = minPartSize
	payload := bytes.Repeat([]byte("abcdefghijklmnopqrstuvwxyz012345"), (6<<20)/32)
	ctx := context.Background()
	wc, err := store.Create(ctx, "name/20060102T150405Z/data/blob.bin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := wc.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := wc.Close(); err != nil {
		t.Fatal(err)
	}
	rc, err := store.Open(ctx, "name/20060102T150405Z/data/blob.bin")
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(rc)
	rc.Close()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("read %d bytes, wrote %d", len(got), len(payload))
	}
}
