package storage

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
	"github.com/olshmore/ytter/pkg/config"
)

// S3 issues presigned PUT URLs against an S3 bucket.
type S3 struct {
	client     *s3.Client
	presigner  *s3.PresignClient
	bucket     string
	publicBase string
}

func NewS3(cfg config.Config) (*S3, error) {
	bucket := strings.TrimSpace(cfg.S3Bucket)
	if bucket == "" {
		return nil, fmt.Errorf("S3_BUCKET is required when STORAGE_DRIVER=s3")
	}
	publicBase := strings.TrimRight(strings.TrimSpace(cfg.S3PublicBaseURL), "/")
	if publicBase == "" {
		return nil, fmt.Errorf("S3_PUBLIC_BASE_URL is required when STORAGE_DRIVER=s3")
	}

	loadOpts := []func(*awsconfig.LoadOptions) error{}
	if region := strings.TrimSpace(cfg.S3Region); region != "" {
		loadOpts = append(loadOpts, awsconfig.WithRegion(region))
	}
	awsCfg, err := awsconfig.LoadDefaultConfig(context.Background(), loadOpts...)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}
	client := s3.NewFromConfig(awsCfg)
	return &S3{
		client:     client,
		presigner:  s3.NewPresignClient(client),
		bucket:     bucket,
		publicBase: publicBase,
	}, nil
}

func (s *S3) Configured() bool { return s != nil && s.bucket != "" && s.publicBase != "" }

func (s *S3) PublicURL(objectKey string) string {
	return s.publicBase + "/" + strings.TrimPrefix(objectKey, "/")
}

func (s *S3) IsPlatformURL(publicURL string) bool {
	return strings.HasPrefix(strings.TrimSpace(publicURL), s.publicBase+"/")
}

func (s *S3) PresignPut(ctx context.Context, locationID uuid.UUID, contentType string, contentLength int64) (*PresignResult, error) {
	ext, ok := ExtForContentType(contentType)
	if !ok {
		return nil, fmt.Errorf("unsupported content type")
	}
	if contentLength <= 0 || contentLength > MaxLogoBytes {
		return nil, fmt.Errorf("invalid content length")
	}
	key := ObjectKey(locationID, ext)
	expires := time.Now().UTC().Add(PresignTTL)
	out, err := s.presigner.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(s.bucket),
		Key:           aws.String(key),
		ContentType:   aws.String(strings.ToLower(strings.TrimSpace(contentType))),
		ContentLength: aws.Int64(contentLength),
	}, s3.WithPresignExpires(PresignTTL))
	if err != nil {
		return nil, fmt.Errorf("presign put: %w", err)
	}
	return &PresignResult{
		UploadURL: out.URL,
		PublicURL: s.PublicURL(key),
		ExpiresAt: expires,
		ObjectKey: key,
	}, nil
}
