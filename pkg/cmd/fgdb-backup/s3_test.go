// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

package main

import (
	"bytes"
	"context"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
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

func TestS3AbortDeletesPartial(t *testing.T) {
	store := fakeS3(t)
	ctx := context.Background()
	wc, err := store.Create(ctx, "name/20060102T150405Z/data/partial.pgcopy")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := wc.Write([]byte("partial row")); err != nil {
		t.Fatal(err)
	}
	if err := wc.(interface{ Abort() error }).Abort(); err != nil {
		t.Fatal(err)
	}
	ok, err := store.Exists(ctx, "name/20060102T150405Z/data/partial.pgcopy")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("aborted object is still visible")
	}
}

func TestS3AbortAfterCancel(t *testing.T) {
	store := fakeS3(t)
	s3s := store.(*s3Store)
	s3s.partSize = minPartSize
	ctx, cancel := context.WithCancel(context.Background())
	wc, err := store.Create(ctx, "name/20060102T150405Z/data/blob.bin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := wc.Write(bytes.Repeat([]byte("a"), minPartSize)); err != nil {
		t.Fatal(err)
	}
	cancel()
	if err := wc.(interface{ Abort() error }).Abort(); err != nil {
		t.Fatal(err)
	}
	out, err := s3s.client.ListMultipartUploads(context.Background(), &s3.ListMultipartUploadsInput{
		Bucket: aws.String(s3s.bucket),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Uploads) != 0 {
		t.Fatalf("cancelled upload left %d multipart uploads", len(out.Uploads))
	}
}

func TestS3PartLimit(t *testing.T) {
	store := fakeS3(t)
	s3s := store.(*s3Store)
	s3s.partSize = minPartSize
	s3s.maxParts = 2
	ctx := context.Background()
	wc, err := store.Create(ctx, "name/20060102T150405Z/data/big.pgcopy")
	if err != nil {
		t.Fatal(err)
	}
	_, err = wc.Write(bytes.Repeat([]byte("a"), minPartSize*2+1))
	if err == nil {
		err = wc.Close()
	}
	if err == nil || !strings.Contains(err.Error(), "stops at") {
		t.Fatalf("expected a multipart limit error, got %v", err)
	}
	ok, existsErr := store.Exists(ctx, "name/20060102T150405Z/data/big.pgcopy")
	if existsErr != nil {
		t.Fatal(existsErr)
	}
	if ok {
		t.Fatal("object that hit the part limit was published")
	}
}

func fakeS3(t *testing.T) Store {
	t.Helper()
	backend := s3mem.New()
	if err := backend.CreateBucket("lab"); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(gofakes3.New(backend).Server())
	t.Cleanup(srv.Close)
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
	return store
}
