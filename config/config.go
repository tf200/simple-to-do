package config

import (
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	MongoURI  string
	JWTSecret string
	Port      string
}

func Load() *Config {
	godotenv.Load()

	cfg := &Config{
		MongoURI:  getEnv("MONGO_URI", "mongodb://localhost:27017"),
		JWTSecret: getEnv("JWT_SECRET", "changeme"),
		Port:      getEnv("PORT", "8080"),
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
