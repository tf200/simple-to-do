package attachment

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// ErrNoObject means the store has nothing under the key.
var ErrNoObject = errors.New("no such object")

// Store is where attachment bytes live. The API never handles them: it signs links that let
// the app upload and download straight to and from the store, and checks on the result.
type Store interface {
	// PresignPut is a link for one upload of exactly `size` bytes of type `mime`. The upload
	// must send the returned headers as they are (they're part of the signature).
	PresignPut(ctx context.Context, key string, size int64, mime string, ttl time.Duration) (string, map[string]string, error)
	// PresignGet is a download link that names the file `filename`.
	PresignGet(ctx context.Context, key, filename, mime string, ttl time.Duration) (string, error)
	// Size of what was uploaded, or ErrNoObject.
	Size(ctx context.Context, key string) (int64, error)
	// Delete removes the object; a missing one is not an error.
	Delete(ctx context.Context, key string) error
}

// S3Config says how to reach the store.
type S3Config struct {
	// Endpoint is where the API itself reaches the store, e.g. http://garage:3900.
	Endpoint string
	// PublicURL is the address in the links handed to apps, e.g. https://s3.example.com.
	// Empty: the store can still clean up, but signs no links (attachments are off).
	PublicURL string
	Region    string
	Bucket    string
	AccessKey string
	SecretKey string
}

// S3Store is a Store on an S3-compatible server (Garage).
type S3Store struct {
	bucket   string
	internal *minio.Client
	// nil when there's no public URL.
	public *minio.Client
}

func NewS3Store(cfg S3Config) (*S3Store, error) {
	internal, err := s3Client(cfg.Endpoint, cfg)
	if err != nil {
		return nil, fmt.Errorf("S3_ENDPOINT: %w", err)
	}
	s := &S3Store{bucket: cfg.Bucket, internal: internal}
	if cfg.PublicURL != "" {
		if s.public, err = s3Client(cfg.PublicURL, cfg); err != nil {
			return nil, fmt.Errorf("S3_PUBLIC_URL: %w", err)
		}
	}
	return s, nil
}

func s3Client(address string, cfg S3Config) (*minio.Client, error) {
	u, err := url.Parse(address)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, fmt.Errorf("%q is not an http(s) URL", address)
	}
	return minio.New(u.Host, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: u.Scheme == "https",
		// Set, so signing a link never asks the server for the bucket's region.
		Region:       cfg.Region,
		BucketLookup: minio.BucketLookupPath,
	})
}

// CanSign reports whether links can be handed out.
func (s *S3Store) CanSign() bool { return s.public != nil }

func (s *S3Store) PresignPut(ctx context.Context, key string, size int64, mime string, ttl time.Duration) (string, map[string]string, error) {
	if s.public == nil {
		return "", nil, ErrDisabled
	}
	headers := map[string]string{
		"Content-Length": strconv.FormatInt(size, 10),
		"Content-Type":   mime,
	}
	signed := http.Header{}
	for k, v := range headers {
		signed.Set(k, v)
	}
	u, err := s.public.PresignHeader(ctx, http.MethodPut, s.bucket, key, ttl, nil, signed)
	if err != nil {
		return "", nil, err
	}
	return u.String(), headers, nil
}

func (s *S3Store) PresignGet(ctx context.Context, key, filename, mime string, ttl time.Duration) (string, error) {
	if s.public == nil {
		return "", ErrDisabled
	}
	params := url.Values{}
	params.Set("response-content-disposition", contentDisposition(filename))
	params.Set("response-content-type", mime)
	u, err := s.public.PresignedGetObject(ctx, s.bucket, key, ttl, params)
	if err != nil {
		return "", err
	}
	return u.String(), nil
}

func (s *S3Store) Size(ctx context.Context, key string) (int64, error) {
	info, err := s.internal.StatObject(ctx, s.bucket, key, minio.StatObjectOptions{})
	if err != nil {
		if minio.ToErrorResponse(err).StatusCode == http.StatusNotFound {
			return 0, ErrNoObject
		}
		return 0, err
	}
	return info.Size, nil
}

func (s *S3Store) Delete(ctx context.Context, key string) error {
	return s.internal.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{})
}

// contentDisposition names the download: a plain ASCII fallback plus the exact UTF-8 name.
func contentDisposition(name string) string {
	fallback := make([]rune, 0, len(name))
	for _, r := range name {
		if r < 0x20 || r > 0x7e || r == '"' || r == '\\' {
			r = '_'
		}
		fallback = append(fallback, r)
	}
	return fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`, string(fallback), rfc5987(name))
}

// rfc5987 percent-encodes everything but letters, digits and -._~ (RFC 5987's safe set).
func rfc5987(s string) string {
	const hex = "0123456789ABCDEF"
	out := make([]byte, 0, len(s)*3)
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '.' || c == '_' || c == '~' {
			out = append(out, c)
			continue
		}
		out = append(out, '%', hex[c>>4], hex[c&15])
	}
	return string(out)
}
