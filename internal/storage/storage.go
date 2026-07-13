package storage

import (
	"context"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/olshmore/ytter/pkg/config"
)

const (
	MaxLogoBytes = 2 * 1024 * 1024
	PresignTTL   = 15 * time.Minute
)

var AllowedLogoContentTypes = map[string]string{
	"image/png":     "png",
	"image/jpeg":    "jpg",
	"image/webp":    "webp",
	"image/svg+xml": "svg",
}

// PresignResult is returned to the client for a direct PUT upload.
type PresignResult struct {
	UploadURL string
	PublicURL string
	ExpiresAt time.Time
	ObjectKey string
}

// ObjectStore issues platform logo upload URLs and validates public URLs.
type ObjectStore interface {
	PresignPut(ctx context.Context, locationID uuid.UUID, contentType string, contentLength int64) (*PresignResult, error)
	IsPlatformURL(publicURL string) bool
	Configured() bool
}

func ExtForContentType(contentType string) (string, bool) {
	ext, ok := AllowedLogoContentTypes[strings.ToLower(strings.TrimSpace(contentType))]
	return ext, ok
}

func ObjectKey(locationID uuid.UUID, ext string) string {
	return path.Join("branding", locationID.String(), "logo", fmt.Sprintf("%s.%s", uuid.NewString(), ext))
}

func NewFromConfig(cfg config.Config) (ObjectStore, error) {
	driver := strings.ToLower(strings.TrimSpace(cfg.StorageDriver))
	if driver == "" {
		driver = "local"
	}
	switch driver {
	case "local":
		return NewLocal(cfg)
	case "s3":
		return NewS3(cfg)
	default:
		return nil, fmt.Errorf("unsupported STORAGE_DRIVER %q", cfg.StorageDriver)
	}
}
