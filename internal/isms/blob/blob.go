// Package blob provides pluggable storage for organization files (branding, evidence, etc.).
// Two backends: local filesystem (default) and S3-compatible object storage.
//
// All keys are scoped by org UUID: {org-uuid}/{path}
package blob

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// Store is the interface for reading and writing organization files.
type Store interface {
	// Put stores data at the given path.
	Put(ctx context.Context, orgUUID, path string, data []byte) error
	// PutStream stores data from a reader with explicit content type.
	PutStream(ctx context.Context, orgUUID, path, contentType string, r io.Reader) error
	// Get returns the data at the given path.
	Get(ctx context.Context, orgUUID, path string) ([]byte, error)
	// Delete removes the data at the given path.
	Delete(ctx context.Context, orgUUID, path string) error
	// Exists returns whether data exists at the given path.
	Exists(ctx context.Context, orgUUID, path string) (bool, error)
	// URL returns a download URL for the given path (presigned for S3, direct serve for local).
	// Returns empty string if the backend doesn't support URLs (local backend).
	URL(ctx context.Context, orgUUID, path string, expiry time.Duration) (string, error)
}

// ---------- Local filesystem backend ----------

// LocalStore stores files under root/{org-uuid}/{path}.
type LocalStore struct {
	root string // e.g. /home/user/isms/data/orgs
}

// NewLocal creates a local file store rooted at dataDir/orgs.
func NewLocal(dataDir string) *LocalStore {
	return &LocalStore{root: filepath.Join(dataDir, "orgs")}
}

func (l *LocalStore) filePath(orgUUID, p string) string {
	return filepath.Join(l.root, orgUUID, filepath.FromSlash(p))
}

func (l *LocalStore) Put(_ context.Context, orgUUID, path string, data []byte) error {
	fp := l.filePath(orgUUID, path)
	if err := os.MkdirAll(filepath.Dir(fp), 0750); err != nil {
		return fmt.Errorf("creating directory: %w", err)
	}
	return os.WriteFile(fp, data, 0640)
}

func (l *LocalStore) PutStream(_ context.Context, orgUUID, path, _ string, r io.Reader) error {
	fp := l.filePath(orgUUID, path)
	if err := os.MkdirAll(filepath.Dir(fp), 0750); err != nil {
		return fmt.Errorf("creating directory: %w", err)
	}
	f, err := os.OpenFile(fp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0640)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, r)
	return err
}

func (l *LocalStore) Get(_ context.Context, orgUUID, path string) ([]byte, error) {
	return os.ReadFile(l.filePath(orgUUID, path))
}

func (l *LocalStore) Delete(_ context.Context, orgUUID, path string) error {
	err := os.Remove(l.filePath(orgUUID, path))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func (l *LocalStore) Exists(_ context.Context, orgUUID, path string) (bool, error) {
	_, err := os.Stat(l.filePath(orgUUID, path))
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

// URL is not supported for local storage — evidence downloads are served directly by the API.
func (l *LocalStore) URL(_ context.Context, _, _ string, _ time.Duration) (string, error) {
	return "", fmt.Errorf("presigned URLs not supported on local storage")
}

// FilePath returns the absolute filesystem path for direct serving. Local-only.
func (l *LocalStore) FilePath(orgUUID, path string) string {
	return l.filePath(orgUUID, path)
}

// ---------- S3-compatible backend ----------

// S3Store stores files in an S3 bucket under {org-uuid}/{path}.
type S3Store struct {
	client        *s3.Client
	presignClient *s3.PresignClient
	bucket        string
}

// S3Config holds configuration for the S3 backend.
// All values come from ISMS_ env vars — no AWS SDK auto-discovery.
type S3Config struct {
	Bucket    string // ISMS_S3_BUCKET
	Region    string // ISMS_S3_REGION
	Endpoint  string // ISMS_S3_ENDPOINT
	AccessKey string // ISMS_S3_ACCESS_KEY
	SecretKey string // ISMS_S3_SECRET_KEY
}

// NewS3 creates an S3-backed store.
func NewS3(cfg S3Config) (*S3Store, error) {
	if cfg.Bucket == "" {
		return nil, fmt.Errorf("ISMS_S3_BUCKET is required")
	}
	if cfg.AccessKey == "" || cfg.SecretKey == "" {
		return nil, fmt.Errorf("ISMS_S3_ACCESS_KEY and ISMS_S3_SECRET_KEY are required")
	}
	if cfg.Endpoint == "" {
		return nil, fmt.Errorf("ISMS_S3_ENDPOINT is required")
	}

	region := cfg.Region
	if region == "" {
		region = "auto"
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(context.Background(),
		awsconfig.WithRegion(region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			cfg.AccessKey, cfg.SecretKey, "",
		)),
	)
	if err != nil {
		return nil, fmt.Errorf("loading config: %w", err)
	}

	s3Opts := []func(*s3.Options){
		func(o *s3.Options) {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
			o.UsePathStyle = true
		},
	}

	client := s3.NewFromConfig(awsCfg, s3Opts...)
	return &S3Store{
		client:        client,
		presignClient: s3.NewPresignClient(client),
		bucket:        cfg.Bucket,
	}, nil
}

func (s *S3Store) key(orgUUID, path string) string {
	return orgUUID + "/" + path
}

func (s *S3Store) Put(ctx context.Context, orgUUID, path string, data []byte) error {
	ct := detectContentType(path)
	return s.PutStream(ctx, orgUUID, path, ct, bytes.NewReader(data))
}

func (s *S3Store) PutStream(ctx context.Context, orgUUID, path, contentType string, r io.Reader) error {
	if contentType == "" {
		contentType = detectContentType(path)
	}
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(s.key(orgUUID, path)),
		Body:        r,
		ContentType: aws.String(contentType),
	})
	if err != nil {
		return fmt.Errorf("S3 put: %w", err)
	}
	return nil
}

func (s *S3Store) Get(ctx context.Context, orgUUID, path string) ([]byte, error) {
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.key(orgUUID, path)),
	})
	if err != nil {
		return nil, fmt.Errorf("S3 get: %w", err)
	}
	defer out.Body.Close()
	return io.ReadAll(out.Body)
}

func (s *S3Store) Delete(ctx context.Context, orgUUID, path string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.key(orgUUID, path)),
	})
	if err != nil {
		return fmt.Errorf("S3 delete: %w", err)
	}
	return nil
}

func (s *S3Store) Exists(ctx context.Context, orgUUID, path string) (bool, error) {
	_, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.key(orgUUID, path)),
	})
	if err != nil {
		var nsk *types.NotFound
		if errors.As(err, &nsk) {
			return false, nil
		}
		var nk *types.NoSuchKey
		if errors.As(err, &nk) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (s *S3Store) URL(ctx context.Context, orgUUID, path string, expiry time.Duration) (string, error) {
	req, err := s.presignClient.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.key(orgUUID, path)),
	}, func(o *s3.PresignOptions) {
		o.Expires = expiry
	})
	if err != nil {
		return "", fmt.Errorf("presigning URL: %w", err)
	}
	return req.URL, nil
}

func detectContentType(path string) string {
	switch {
	case strings.HasSuffix(path, ".png"):
		return "image/png"
	case strings.HasSuffix(path, ".svg"):
		return "image/svg+xml"
	case strings.HasSuffix(path, ".ico"):
		return "image/x-icon"
	case strings.HasSuffix(path, ".jpg"), strings.HasSuffix(path, ".jpeg"):
		return "image/jpeg"
	case strings.HasSuffix(path, ".pdf"):
		return "application/pdf"
	default:
		return "application/octet-stream"
	}
}
