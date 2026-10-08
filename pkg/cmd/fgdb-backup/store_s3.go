// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

const minPartSize = 5 << 20

type s3Store struct {
	client     *s3.Client
	cfg        aws.Config
	bucket     string
	root       string
	region     string
	endpoint   string
	sse        types.ServerSideEncryption
	kmsKeyID   string
	partSize   int
	importAuth string
}

func newS3Store(ctx context.Context, loc Location) (*s3Store, error) {
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(loc.Region))
	if err != nil {
		return nil, err
	}
	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		if loc.Endpoint != "" {
			o.BaseEndpoint = aws.String(loc.Endpoint)
			o.UsePathStyle = true
		}
	})
	st := &s3Store{
		client:     client,
		cfg:        cfg,
		bucket:     loc.Bucket,
		root:       strings.Trim(loc.Root, "/"),
		region:     loc.Region,
		endpoint:   loc.Endpoint,
		partSize:   8 << 20,
		importAuth: loc.ImportAuth,
	}
	switch loc.SSE {
	case "AES256":
		st.sse = types.ServerSideEncryptionAes256
	case "aws:kms":
		st.sse = types.ServerSideEncryptionAwsKms
		st.kmsKeyID = loc.KMSKeyID
	}
	return st, nil
}

func (s *s3Store) key(rel string) string {
	rel = strings.Trim(strings.ReplaceAll(rel, "\\", "/"), "/")
	if s.root == "" {
		return rel
	}
	if rel == "" {
		return s.root
	}
	return s.root + "/" + rel
}

func (s *s3Store) applySSE(sse *types.ServerSideEncryption, kms **string) {
	if s.sse == "" {
		return
	}
	*sse = s.sse
	if s.kmsKeyID != "" {
		*kms = aws.String(s.kmsKeyID)
	}
}

func (s *s3Store) Create(ctx context.Context, rel string) (io.WriteCloser, error) {
	return &s3Writer{
		ctx:   ctx,
		store: s,
		key:   s.key(rel),
		parts: newPartWriter(s.partSize, nil),
	}, nil
}

type s3Writer struct {
	ctx      context.Context
	store    *s3Store
	key      string
	parts    *partWriter
	uploadID string
	done     []types.CompletedPart
	putDone  bool
	closed   bool
}

func (w *s3Writer) Write(p []byte) (int, error) {
	if w.parts.flush == nil {
		w.parts.flush = w.flush
	}
	return w.parts.Write(p)
}

func (w *s3Writer) flush(part []byte) error {
	// A short buffer is the whole object when multipart has not started.
	// A short buffer after that is the last part of a multipart upload.
	if w.uploadID == "" && len(part) < w.store.partSize {
		return w.put(part)
	}
	if w.uploadID == "" {
		if err := w.start(); err != nil {
			return err
		}
	}
	num := int32(len(w.done) + 1)
	in := &s3.UploadPartInput{
		Bucket:     aws.String(w.store.bucket),
		Key:        aws.String(w.key),
		UploadId:   aws.String(w.uploadID),
		PartNumber: aws.Int32(num),
		Body:       bytes.NewReader(part),
	}
	out, err := w.store.client.UploadPart(w.ctx, in)
	if err != nil {
		_ = w.abort()
		return err
	}
	w.done = append(w.done, types.CompletedPart{
		ETag:       out.ETag,
		PartNumber: aws.Int32(num),
	})
	return nil
}

func (w *s3Writer) put(part []byte) error {
	in := &s3.PutObjectInput{
		Bucket: aws.String(w.store.bucket),
		Key:    aws.String(w.key),
		Body:   bytes.NewReader(part),
	}
	w.store.applySSE(&in.ServerSideEncryption, &in.SSEKMSKeyId)
	_, err := w.store.client.PutObject(w.ctx, in)
	if err != nil {
		return err
	}
	w.putDone = true
	return nil
}

func (w *s3Writer) start() error {
	in := &s3.CreateMultipartUploadInput{
		Bucket: aws.String(w.store.bucket),
		Key:    aws.String(w.key),
	}
	w.store.applySSE(&in.ServerSideEncryption, &in.SSEKMSKeyId)
	out, err := w.store.client.CreateMultipartUpload(w.ctx, in)
	if err != nil {
		return err
	}
	w.uploadID = aws.ToString(out.UploadId)
	return nil
}

func (w *s3Writer) abort() error {
	if w.uploadID == "" {
		return nil
	}
	_, err := w.store.client.AbortMultipartUpload(w.ctx, &s3.AbortMultipartUploadInput{
		Bucket:   aws.String(w.store.bucket),
		Key:      aws.String(w.key),
		UploadId: aws.String(w.uploadID),
	})
	return err
}

func (w *s3Writer) Close() error {
	if w.closed {
		return nil
	}
	w.closed = true
	if w.parts.flush == nil {
		w.parts.flush = w.flush
	}
	// A single short buffer should be PutObject, not a multipart of one part.
	// partWriter.Close emits whatever is left. If multipart has not started
	// and the buffer is shorter than partSize, flush() puts the object.
	if err := w.parts.Close(); err != nil {
		return err
	}
	if w.uploadID == "" {
		if !w.putDone {
			return w.put(nil)
		}
		return nil
	}
	_, err := w.store.client.CompleteMultipartUpload(w.ctx, &s3.CompleteMultipartUploadInput{
		Bucket:   aws.String(w.store.bucket),
		Key:      aws.String(w.key),
		UploadId: aws.String(w.uploadID),
		MultipartUpload: &types.CompletedMultipartUpload{
			Parts: w.done,
		},
	})
	if err != nil {
		_ = w.abort()
		return err
	}
	return nil
}

func (s *s3Store) Open(ctx context.Context, rel string) (io.ReadCloser, error) {
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.key(rel)),
	})
	if err != nil {
		return nil, err
	}
	return out.Body, nil
}

func (s *s3Store) Exists(ctx context.Context, rel string) (bool, error) {
	_, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.key(rel)),
	})
	if err == nil {
		return true, nil
	}
	if isNotFound(err) {
		return false, nil
	}
	return false, err
}

func (s *s3Store) ListManifests(ctx context.Context, rel string) ([]string, error) {
	prefix := s.listPrefix(rel)
	var out []string
	var token *string
	for {
		page, err := s.listPage(ctx, prefix, token)
		if err != nil {
			return nil, err
		}
		names, err := s.manifestNames(ctx, rel, prefix, page.CommonPrefixes)
		if err != nil {
			return nil, err
		}
		out = append(out, names...)
		if !aws.ToBool(page.IsTruncated) {
			return out, nil
		}
		token = page.NextContinuationToken
	}
}

func (s *s3Store) listPrefix(rel string) string {
	prefix := s.key(rel)
	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	return prefix
}

func (s *s3Store) listPage(ctx context.Context, prefix string, token *string) (*s3.ListObjectsV2Output, error) {
	return s.client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket:            aws.String(s.bucket),
		Prefix:            aws.String(prefix),
		Delimiter:         aws.String("/"),
		ContinuationToken: token,
	})
}

func (s *s3Store) manifestNames(ctx context.Context, rel, prefix string, prefixes []types.CommonPrefix) ([]string, error) {
	var out []string
	for _, cp := range prefixes {
		name, ok, err := s.completeManifest(ctx, rel, prefix, aws.ToString(cp.Prefix))
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, name)
		}
	}
	return out, nil
}

func (s *s3Store) completeManifest(ctx context.Context, rel, prefix, child string) (string, bool, error) {
	name := strings.Trim(strings.TrimPrefix(child, prefix), "/")
	if name == "" || strings.Contains(name, "/") {
		return "", false, nil
	}
	ok, err := s.Exists(ctx, strings.Trim(rel+"/"+name+"/manifest.json", "/"))
	return name, ok, err
}

func isNotFound(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "NotFound") || strings.Contains(msg, "status code: 404") || strings.Contains(msg, "NoSuchKey")
}

// importURL is the s3 URL the database itself reads during IMPORT.
// Credentials are added only for AUTH=specified. They are not written to the manifest.
func (s *s3Store) importURL(ctx context.Context, rel string) (string, error) {
	u := url.URL{
		Scheme: "s3",
		Host:   s.bucket,
		Path:   "/" + s.key(rel),
	}
	q := url.Values{}
	q.Set("AWS_REGION", s.region)
	if s.endpoint != "" {
		q.Set("AWS_ENDPOINT", s.endpoint)
	}
	auth := s.importAuth
	if auth == "" || auth == "auto" {
		if os.Getenv("AWS_ACCESS_KEY_ID") != "" {
			auth = "specified"
		} else {
			auth = "implicit"
		}
	}
	switch auth {
	case "implicit":
		q.Set("AUTH", "implicit")
	case "specified":
		creds, err := s.cfg.Credentials.Retrieve(ctx)
		if err != nil {
			return "", fmt.Errorf("read AWS credentials for IMPORT: %w", err)
		}
		if creds.AccessKeyID == "" || creds.SecretAccessKey == "" {
			return "", fmt.Errorf("AUTH=specified needs AWS access key credentials; the default chain did not provide them")
		}
		q.Set("AUTH", "specified")
		q.Set("AWS_ACCESS_KEY_ID", creds.AccessKeyID)
		q.Set("AWS_SECRET_ACCESS_KEY", creds.SecretAccessKey)
		if creds.SessionToken != "" {
			q.Set("AWS_SESSION_TOKEN", creds.SessionToken)
		}
	default:
		return "", fmt.Errorf("unknown s3 import auth %q", auth)
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}
