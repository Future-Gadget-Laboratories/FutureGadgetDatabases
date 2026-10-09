// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"hash"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	smithymiddleware "github.com/aws/smithy-go/middleware"
	smithyhttp "github.com/aws/smithy-go/transport/http"
)

const (
	minPartSize       = 5 << 20
	maxS3Parts        = 10000
	ifNoneMatchHeader = "If-None-Match"
)

type s3Store struct {
	client      *s3.Client
	cfg         aws.Config
	bucket      string
	root        string
	region      string
	endpoint    string
	sse         types.ServerSideEncryption
	kmsKeyID    string
	partSize    int
	maxParts    int
	importAuth  string
	allowUnsafe bool
}

func newS3Store(ctx context.Context, loc Location) (*s3Store, error) {
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(loc.Region))
	if err != nil {
		return nil, err
	}
	if credentials, credentialErr := cfg.Credentials.Retrieve(ctx); credentialErr != nil {
		logf("s3: could not determine credential source: %s", scrubSecrets(credentialErr.Error()))
	} else {
		logf("s3: credentials from %s", credentials.Source)
	}
	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		if loc.Endpoint != "" {
			o.BaseEndpoint = aws.String(loc.Endpoint)
			o.UsePathStyle = true
		}
	})
	st := &s3Store{
		client:      client,
		cfg:         cfg,
		bucket:      loc.Bucket,
		root:        strings.Trim(loc.Root, "/"),
		region:      loc.Region,
		endpoint:    loc.Endpoint,
		partSize:    8 << 20,
		maxParts:    maxS3Parts,
		importAuth:  loc.ImportAuth,
		allowUnsafe: loc.AllowUnsafeOverwrite,
	}
	switch loc.SSE {
	case "AES256":
		st.sse = types.ServerSideEncryptionAes256
	case "aws:kms":
		st.sse = types.ServerSideEncryptionAwsKms
		st.kmsKeyID = loc.KMSKeyID
	}
	if err := st.checkConditionalWrites(ctx); err != nil {
		return nil, err
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
		ctx:      ctx,
		store:    s,
		key:      s.key(rel),
		parts:    newPartWriter(s.partSize, nil),
		checksum: sha256.New(),
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
	checksum hash.Hash
}

func (w *s3Writer) Write(p []byte) (int, error) {
	if w.parts.flush == nil {
		w.parts.flush = w.flush
	}
	n, err := w.parts.Write(p)
	if n > 0 {
		_, _ = w.checksum.Write(p[:n])
	}
	return n, err
}

func (s *s3Store) partLimit() int {
	if s.maxParts > 0 {
		return s.maxParts
	}
	return maxS3Parts
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
	if len(w.done) >= w.store.partLimit() {
		_ = w.abort()
		w.uploadID = ""
		limit := w.store.partLimit()
		return fmt.Errorf("S3 multipart upload stops at %d parts of %d bytes (about %d bytes). This object would exceed that limit. Raise --part-size or pass --split-rows so each file stays under the limit", limit, w.store.partSize, int64(limit)*int64(w.store.partSize))
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
	_, err := w.store.client.PutObject(w.ctx, in, putHeader(ifNoneMatchHeader, "*"))
	if err != nil {
		if isPreconditionFailed(err) && w.store.objectMatches(w.ctx, w.key, sha256Bytes(part)) {
			w.putDone = true
			return nil
		}
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

// cleanupContext stays usable after SIGINT or SIGTERM cancels w.ctx. Aborting
// on the cancelled context leaves the uploaded parts in the bucket.
func (w *s3Writer) cleanupContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(w.ctx), 30*time.Second)
}

func (w *s3Writer) abort() error {
	if w.uploadID == "" {
		return nil
	}
	ctx, cancel := w.cleanupContext()
	defer cancel()
	_, err := w.store.client.AbortMultipartUpload(ctx, &s3.AbortMultipartUploadInput{
		Bucket:   aws.String(w.store.bucket),
		Key:      aws.String(w.key),
		UploadId: aws.String(w.uploadID),
	})
	return err
}

// Abort drops an unfinished upload and deletes a short object that was
// already put. The final key is not left behind as a finished file.
func (w *s3Writer) Abort() error {
	if w.closed {
		return nil
	}
	w.closed = true
	var err error
	if w.uploadID != "" {
		err = w.abort()
		w.uploadID = ""
	}
	if w.putDone {
		ctx, cancel := w.cleanupContext()
		defer cancel()
		_, delErr := w.store.client.DeleteObject(ctx, &s3.DeleteObjectInput{
			Bucket: aws.String(w.store.bucket),
			Key:    aws.String(w.key),
		})
		w.putDone = false
		if err == nil {
			err = delErr
		}
	}
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
		if w.uploadID != "" {
			_ = w.abort()
			w.uploadID = ""
		}
		if w.putDone {
			ctx, cancel := w.cleanupContext()
			_, _ = w.store.client.DeleteObject(ctx, &s3.DeleteObjectInput{
				Bucket: aws.String(w.store.bucket),
				Key:    aws.String(w.key),
			})
			cancel()
			w.putDone = false
		}
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
	}, putHeader(ifNoneMatchHeader, "*"))
	if err != nil {
		if isPreconditionFailed(err) && w.store.objectMatches(w.ctx, w.key, w.checksum.Sum(nil)) {
			return nil
		}
		_ = w.abort()
		return err
	}
	return nil
}

func sha256Bytes(data []byte) []byte {
	sum := sha256.Sum256(data)
	return sum[:]
}

func (s *s3Store) objectMatches(ctx context.Context, key string, want []byte) bool {
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return false
	}
	defer out.Body.Close()
	h := sha256.New()
	if _, err := io.Copy(h, out.Body); err != nil {
		return false
	}
	return bytes.Equal(h.Sum(nil), want)
}

func (s *s3Store) checkConditionalWrites(ctx context.Context) error {
	key := s.key(".fgdb-if-none-match-" + newBackupID(time.Now()))
	body := bytes.NewReader([]byte("probe"))
	if _, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
		Body:   body,
	}, putHeader(ifNoneMatchHeader, "*")); err != nil {
		return fmt.Errorf("check S3 conditional writes: %w", err)
	}
	defer func() {
		_, _ = s.client.DeleteObject(context.WithoutCancel(ctx), &s3.DeleteObjectInput{
			Bucket: aws.String(s.bucket),
			Key:    aws.String(key),
		})
	}()
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
		Body:   bytes.NewReader([]byte("probe")),
	}, putHeader(ifNoneMatchHeader, "*"))
	if err == nil {
		if s.allowUnsafe {
			logf("S3 endpoint does not enforce If-None-Match; unsafe overwrite mode is enabled")
			return nil
		}
		return fmt.Errorf("S3 endpoint does not enforce If-None-Match; set allow_unsafe_overwrite only when overwrites are acceptable")
	}
	if !isPreconditionFailed(err) {
		return fmt.Errorf("S3 conditional write probe failed: %w", err)
	}
	return nil
}

func isPreconditionFailed(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "412") || strings.Contains(msg, "preconditionfailed")
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

func (s *s3Store) Size(ctx context.Context, rel string) (int64, error) {
	out, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.key(rel)),
	})
	if err != nil {
		return 0, err
	}
	return aws.ToInt64(out.ContentLength), nil
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

func (s *s3Store) delete(ctx context.Context, rel string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.key(rel)),
	})
	return err
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

// implicitImport is true when the database node can read S3 without this
// process putting credentials in the IMPORT statement.
func (s *s3Store) implicitImport() bool {
	auth := s.importAuth
	if auth == "" || auth == "auto" || auth == "served" || auth == "specified" {
		return false
	}
	return auth == "implicit"
}

// importURL is the s3 URL the database itself reads. It never contains
// access keys. Explicit credentials are served over HTTP by the tool instead,
// so job records and logs cannot see them.
func (s *s3Store) importURL(_ context.Context, rel string) (string, error) {
	if !s.implicitImport() {
		return "", fmt.Errorf("refusing to put AWS credentials in an IMPORT statement")
	}
	u := url.URL{
		Scheme: "s3",
		Host:   s.bucket,
		Path:   "/" + s.key(rel),
	}
	q := url.Values{}
	q.Set("AWS_REGION", s.region)
	q.Set("AUTH", "implicit")
	if s.endpoint != "" {
		q.Set("AWS_ENDPOINT", s.endpoint)
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// putLatest writes latest.json only when candidate is newer than the object
// already there. If-Match / If-None-Match stop an older backup from winning
// a race against a newer one.
func (s *s3Store) putLatest(ctx context.Context, rel string, ptr LatestPointer) error {
	body, err := json.MarshalIndent(ptr, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	key := s.key(rel)
	var last error
	for attempt := 0; attempt < 5; attempt++ {
		etag, existing, found, err := s.readLatest(ctx, rel)
		if err != nil {
			return err
		}
		if found && !newerBackup(ptr.Timestamp, existing) {
			logf("leaving latest at %s; %s is not newer", existing, ptr.Timestamp)
			return nil
		}
		header, value := ifNoneMatchHeader, "*"
		if found {
			header, value = "If-Match", etag
		}
		in := &s3.PutObjectInput{
			Bucket: aws.String(s.bucket),
			Key:    aws.String(key),
			Body:   bytes.NewReader(body),
		}
		s.applySSE(&in.ServerSideEncryption, &in.SSEKMSKeyId)
		_, err = s.client.PutObject(ctx, in, putHeader(header, value))
		if err == nil {
			return nil
		}
		last = err
	}
	return fmt.Errorf("update latest pointer: %w", last)
}

func (s *s3Store) readLatest(ctx context.Context, rel string) (etag, timestamp string, found bool, err error) {
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.key(rel)),
	})
	if err != nil {
		if isNotFound(err) {
			return "", "", false, nil
		}
		return "", "", false, err
	}
	defer out.Body.Close()
	var ptr LatestPointer
	if decErr := json.NewDecoder(out.Body).Decode(&ptr); decErr != nil {
		return "", "", false, decErr
	}
	return aws.ToString(out.ETag), ptr.Timestamp, true, nil
}

func putHeader(key, value string) func(*s3.Options) {
	return func(o *s3.Options) {
		o.APIOptions = append(o.APIOptions, func(stack *smithymiddleware.Stack) error {
			return stack.Finalize.Add(
				smithymiddleware.FinalizeMiddlewareFunc("fgdbBackupHeader", func(
					ctx context.Context,
					in smithymiddleware.FinalizeInput,
					next smithymiddleware.FinalizeHandler,
				) (smithymiddleware.FinalizeOutput, smithymiddleware.Metadata, error) {
					if req, ok := in.Request.(*smithyhttp.Request); ok {
						req.Header.Set(key, value)
					}
					return next.HandleFinalize(ctx, in)
				}),
				smithymiddleware.Before,
			)
		})
	}
}
