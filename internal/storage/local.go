package storage

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/olshmore/ytter/pkg/config"
)

type localUploadToken struct {
	Key         string `json:"key"`
	ContentType string `json:"ct"`
	MaxBytes    int64  `json:"mb"`
	Exp         int64  `json:"exp"`
}

// Local stores branding assets on disk and serves them via HTTP handlers.
type Local struct {
	dir        string
	publicBase string
	secret     []byte
}

func NewLocal(cfg config.Config) (*Local, error) {
	dir := strings.TrimSpace(cfg.StorageLocalDir)
	if dir == "" {
		dir = "./data/branding"
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create storage dir: %w", err)
	}
	publicBase := strings.TrimRight(strings.TrimSpace(cfg.StoragePublicBaseURL), "/")
	if publicBase == "" {
		publicBase = "http://localhost:8080/v1/public/media"
	}
	secret := []byte(cfg.TokenSymmetricKey)
	if len(secret) == 0 {
		return nil, fmt.Errorf("TOKEN_SYMMETRIC_KEY required for local storage signing")
	}
	return &Local{dir: dir, publicBase: publicBase, secret: secret}, nil
}

func (l *Local) Configured() bool { return l != nil }

func (l *Local) PublicURL(objectKey string) string {
	return l.publicBase + "/" + strings.TrimPrefix(objectKey, "/")
}

func (l *Local) IsPlatformURL(publicURL string) bool {
	u := strings.TrimSpace(publicURL)
	return strings.HasPrefix(u, l.publicBase+"/")
}

func (l *Local) PresignPut(_ context.Context, locationID uuid.UUID, contentType string, contentLength int64) (*PresignResult, error) {
	ext, ok := ExtForContentType(contentType)
	if !ok {
		return nil, fmt.Errorf("unsupported content type")
	}
	if contentLength <= 0 || contentLength > MaxLogoBytes {
		return nil, fmt.Errorf("invalid content length")
	}
	key := ObjectKey(locationID, ext)
	expires := time.Now().UTC().Add(PresignTTL)
	tok, err := l.signToken(localUploadToken{
		Key:         key,
		ContentType: strings.ToLower(strings.TrimSpace(contentType)),
		MaxBytes:    contentLength,
		Exp:         expires.Unix(),
	})
	if err != nil {
		return nil, err
	}
	return &PresignResult{
		UploadURL: l.publicBase + "/upload/" + tok,
		PublicURL: l.PublicURL(key),
		ExpiresAt: expires,
		ObjectKey: key,
	}, nil
}

func (l *Local) signToken(payload localUploadToken) (string, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, l.secret)
	_, _ = mac.Write(raw)
	sig := mac.Sum(nil)
	return base64.RawURLEncoding.EncodeToString(raw) + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

func (l *Local) parseToken(token string) (*localUploadToken, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid token")
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, fmt.Errorf("invalid token payload")
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("invalid token signature")
	}
	mac := hmac.New(sha256.New, l.secret)
	_, _ = mac.Write(raw)
	if !hmac.Equal(sig, mac.Sum(nil)) {
		return nil, fmt.Errorf("invalid token signature")
	}
	var payload localUploadToken
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("invalid token payload")
	}
	if time.Now().UTC().Unix() > payload.Exp {
		return nil, fmt.Errorf("upload token expired")
	}
	return &payload, nil
}

func (l *Local) diskPath(objectKey string) (string, error) {
	clean := filepath.Clean("/" + objectKey)
	clean = strings.TrimPrefix(clean, "/")
	if clean == "" || strings.Contains(clean, "..") {
		return "", fmt.Errorf("invalid object key")
	}
	full := filepath.Join(l.dir, filepath.FromSlash(clean))
	if !strings.HasPrefix(full, filepath.Clean(l.dir)+string(os.PathSeparator)) && full != filepath.Clean(l.dir) {
		return "", fmt.Errorf("invalid object key")
	}
	return full, nil
}

// HandleMedia serves PUT uploads and GET media for the local driver.
func (l *Local) HandleMedia(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/v1/public/media")
	path = strings.TrimPrefix(path, "/")

	switch {
	case r.Method == http.MethodPut && strings.HasPrefix(path, "upload/"):
		l.handleUpload(w, r, strings.TrimPrefix(path, "upload/"))
	case r.Method == http.MethodGet && path != "" && !strings.HasPrefix(path, "upload/"):
		l.handleGet(w, r, path)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (l *Local) handleUpload(w http.ResponseWriter, r *http.Request, token string) {
	payload, err := l.parseToken(token)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	ct := strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Type")))
	if ct == "" {
		ct = payload.ContentType
	}
	if ct != payload.ContentType {
		http.Error(w, "content type mismatch", http.StatusBadRequest)
		return
	}
	limited := io.LimitReader(r.Body, payload.MaxBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}
	if int64(len(data)) > payload.MaxBytes {
		http.Error(w, "file too large", http.StatusBadRequest)
		return
	}
	full, err := l.diskPath(payload.Key)
	if err != nil {
		http.Error(w, "invalid key", http.StatusBadRequest)
		return
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		http.Error(w, "failed to store file", http.StatusInternalServerError)
		return
	}
	if err := os.WriteFile(full, data, 0o644); err != nil {
		http.Error(w, "failed to store file", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (l *Local) handleGet(w http.ResponseWriter, r *http.Request, objectKey string) {
	full, err := l.diskPath(objectKey)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	f, err := os.Open(full)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil || stat.IsDir() {
		http.NotFound(w, r)
		return
	}
	ext := strings.ToLower(filepath.Ext(full))
	switch ext {
	case ".png":
		w.Header().Set("Content-Type", "image/png")
	case ".jpg", ".jpeg":
		w.Header().Set("Content-Type", "image/jpeg")
	case ".webp":
		w.Header().Set("Content-Type", "image/webp")
	case ".svg":
		w.Header().Set("Content-Type", "image/svg+xml")
	default:
		w.Header().Set("Content-Type", "application/octet-stream")
	}
	w.Header().Set("Cache-Control", "public, max-age=86400")
	http.ServeContent(w, r, filepath.Base(full), stat.ModTime(), f)
}
