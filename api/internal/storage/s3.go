// Package storage guarda os arquivos XML enviados.
//
// A interface Storage existe para desacoplar a regra de negócio do provedor:
// hoje é S3; amanhã pode ser Google Cloud Storage implementando o mesmo método.
// Nos testes usamos uma implementação falsa em memória.
package storage

import (
	"bytes"
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type Storage interface {
	Put(ctx context.Context, key string, data []byte, contentType string) error
}

type S3Storage struct {
	client *s3.Client
	bucket string
}

func NewS3(client *s3.Client, bucket string) *S3Storage {
	return &S3Storage{client: client, bucket: bucket}
}

func (s *S3Storage) Put(ctx context.Context, key string, data []byte, contentType string) error {
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(data),
		ContentType: aws.String(contentType),
	})
	if err != nil {
		return fmt.Errorf("s3 put %s: %w", key, err)
	}
	return nil
}
