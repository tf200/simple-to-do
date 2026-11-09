package config

import (
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	MongoURI  string
	JWTSecret string
	Port      string
	S3        S3
}

// S3 is the attachment store. Without an endpoint and keys, attachments are off; without a
// public URL they're off too, but the store is still used to clean up.
type S3 struct {
	Endpoint  string
	PublicURL string
	Region    string
	Bucket    string
	AccessKey string
	SecretKey string
}

// Configured reports whether there's a store at all.
func (s S3) Configured() bool {
	return s.Endpoint != "" && s.AccessKey != "" && s.SecretKey != ""
}

func Load() *Config {
	godotenv.Load()

	cfg := &Config{
		MongoURI:  getEnv("MONGO_URI", "mongodb://localhost:27017"),
		JWTSecret: getEnv("JWT_SECRET", "changeme"),
		Port:      getEnv("PORT", "8080"),
		S3: S3{
			Endpoint:  os.Getenv("S3_ENDPOINT"),
			PublicURL: os.Getenv("S3_PUBLIC_URL"),
			Region:    getEnv("S3_REGION", "garage"),
			Bucket:    getEnv("S3_BUCKET", "attachments"),
			AccessKey: os.Getenv("S3_ACCESS_KEY"),
			SecretKey: os.Getenv("S3_SECRET_KEY"),
		},
	}

	if cfg.JWTSecret == "changeme" {
		panic("JWT_SECRET must be set in production")
	}

	return cfg
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
