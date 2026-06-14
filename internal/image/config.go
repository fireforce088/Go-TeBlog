package image

import (
	"os"
	"strconv"
	"sync"
	"time"
)

const (
	defaultImageStorageDir    = "/data/blog-images"
	defaultImagePublicPrefix  = "/blog-images"
	defaultImageMaxImages     = 20
	defaultImageMaxBytes      = 15 * 1024 * 1024
	defaultImageMaxConcurrent = 3
	defaultImageTimeout       = 60 * time.Second
)

type Config struct {
	StorageDir      string
	PublicPrefix    string
	Enabled         bool
	MaxImages       int
	MaxBytes        int64
	MaxConcurrent   int
	Timeout         time.Duration
	SearchEnabled   bool
	SearchWorkerURL string
}

var (
	Cfg     *Config
	cfgOnce sync.Once
)

func GetConfig() *Config {
	cfgOnce.Do(func() {
		Cfg = LoadConfig()
	})
	return Cfg
}

func LoadConfig() *Config {
	return &Config{
		StorageDir:      envString("IMAGE_STORAGE_DIR", defaultImageStorageDir),
		PublicPrefix:    envString("IMAGE_PUBLIC_PREFIX", defaultImagePublicPrefix),
		Enabled:         envBoolDefaultTrue("IMAGE_LOCALIZE_ENABLED"),
		MaxImages:       envInt("IMAGE_LOCALIZE_MAX_IMAGES", defaultImageMaxImages),
		MaxBytes:        int64(envInt("IMAGE_LOCALIZE_MAX_SIZE_MB", 15)) * 1024 * 1024,
		MaxConcurrent:   envInt("IMAGE_LOCALIZE_MAX_CONCURRENT", defaultImageMaxConcurrent),
		Timeout:         time.Duration(envInt("IMAGE_LOCALIZE_TIMEOUT_SEC", int(defaultImageTimeout/time.Second))) * time.Second,
		SearchEnabled:   envBoolDefaultTrue("IMAGE_SEARCH_ENABLED"),
		SearchWorkerURL: os.Getenv("IMAGE_SEARCH_WORKER_URL"),
	}
}

func envString(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func envBoolDefaultTrue(key string) bool {
	return os.Getenv(key) != "false"
}
