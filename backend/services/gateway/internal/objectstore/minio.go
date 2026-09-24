package objectstore

import (
	"context"
	"fmt"
	"io"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type Store struct {
	client *minio.Client
}

type Config struct {
	Endpoint  string
	AccessKey string
	SecretKey string
	UseSSL    bool
}

func New(cfg Config) (*Store, error) {
	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("create minio client: %w", err)
	}
	return &Store{client: client}, nil
}

func (s *Store) Upload(ctx context.Context, bucket, key string, r io.Reader, size int64, contentType string) error {
	_, err := s.client.PutObject(ctx, bucket, key, r, size, minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return fmt.Errorf("put object %s/%s: %w", bucket, key, err)
	}
	return nil
}

// Remove deletes an object from the bucket. Deleting a nonexistent object is
// not an error (S3 semantics), so Remove is idempotent - safe to call again
// as part of a compensation path.
func (s *Store) Remove(ctx context.Context, bucket, key string) error {
	if err := s.client.RemoveObject(ctx, bucket, key, minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("remove object %s/%s: %w", bucket, key, err)
	}
	return nil
}

func (s *Store) Ping(ctx context.Context) error {
	if _, err := s.client.ListBuckets(ctx); err != nil {
		return fmt.Errorf("list buckets: %w", err)
	}
	return nil
}
