package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

var (
	minioEndpoint    string
	minioAccessKey   string
	minioSecretKey   string
	minioBucket      string
	minioPublicURL   string
	minioInitialized bool
)

func InitMinIO() {
	if minioInitialized {
		return
	}

	minioEndpoint = strings.TrimSpace(os.Getenv("MINIO_ENDPOINT"))
	minioAccessKey = strings.TrimSpace(os.Getenv("MINIO_ACCESS_KEY"))
	minioSecretKey = strings.TrimSpace(os.Getenv("MINIO_SECRET_KEY"))
	minioBucket = strings.TrimSpace(os.Getenv("MINIO_BUCKET"))
	minioPublicURL = strings.TrimRight(strings.TrimSpace(os.Getenv("MINIO_PUBLIC_URL")), "/")
	if minioBucket == "" {
		minioBucket = "blog-images"
	}
	if minioEndpoint == "" || minioAccessKey == "" || minioSecretKey == "" || minioPublicURL == "" {
		log.Println("[MinIO] missing MINIO_ENDPOINT/MINIO_ACCESS_KEY/MINIO_SECRET_KEY/MINIO_PUBLIC_URL; uploads will use local storage")
		return
	}

	cmd := exec.Command("curl", "-s", "-o", "/dev/null", "-w", "%{http_code}",
		"--connect-timeout", "5",
		minioEndpoint+"/minio/health/live")
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("[MinIO] health check failed: %v (output: %s)", err, strings.TrimSpace(string(output)))
		log.Println("[MinIO] uploads will use local storage")
		return
	}

	status := strings.TrimSpace(string(output))
	if status == "200" {
		minioInitialized = true
		log.Println("[MinIO] connection ok; images will sync to MinIO")
		return
	}

	log.Printf("[MinIO] health check returned %s; uploads will use local storage", status)
}

func uploadToMinIO(localAbsPath, relativePath string) string {
	if !minioInitialized {
		return ""
	}

	objName := strings.TrimPrefix(relativePath, "/blog/")
	contentType := detectContentType(localAbsPath)
	minioURL := fmt.Sprintf("%s/%s", minioEndpoint, filepath.ToSlash(filepath.Join(minioBucket, objName)))

	cmd := exec.Command("curl", "-s", "-o", "/dev/null", "-w", "%{http_code}",
		"-X", "PUT",
		"-T", localAbsPath,
		"--aws-sigv4", "aws:amz:auto:s3",
		"--user", fmt.Sprintf("%s:%s", minioAccessKey, minioSecretKey),
		"-H", fmt.Sprintf("Content-Type: %s", contentType),
		"--connect-timeout", "10",
		"--max-time", "30",
		minioURL)

	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("[MinIO] upload failed (%s): %v", objName, err)
		return ""
	}

	status := strings.TrimSpace(string(output))
	if status == "200" {
		publicURL := fmt.Sprintf("%s/%s", minioPublicURL, objName)
		log.Printf("[MinIO] uploaded: %s", publicURL)
		return publicURL
	}

	log.Printf("[MinIO] upload returned %s (file: %s)", status, objName)
	return ""
}

func detectContentType(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".svg":
		return "image/svg+xml"
	case ".bmp":
		return "image/bmp"
	case ".ico":
		return "image/x-icon"
	default:
		return "application/octet-stream"
	}
}

func GetMinioPublicURL(relativePath string) string {
	if !minioInitialized {
		return ""
	}
	objName := strings.TrimPrefix(relativePath, "/blog/")
	return fmt.Sprintf("%s/%s?t=%d", minioPublicURL, objName, time.Now().Unix())
}

func DeleteFromMinIO(relativePath string) bool {
	if !minioInitialized {
		return false
	}

	objName := strings.TrimPrefix(relativePath, "/blog/")
	minioURL := fmt.Sprintf("%s/%s", minioEndpoint, filepath.ToSlash(filepath.Join(minioBucket, objName)))

	cmd := exec.Command("curl", "-s", "-o", "/dev/null", "-w", "%{http_code}",
		"-X", "DELETE",
		"--aws-sigv4", "aws:amz:auto:s3",
		"--user", fmt.Sprintf("%s:%s", minioAccessKey, minioSecretKey),
		"--connect-timeout", "10",
		"--max-time", "15",
		minioURL)

	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("[MinIO] delete failed (%s): %v", objName, err)
		return false
	}

	status := strings.TrimSpace(string(output))
	if status == "204" || status == "200" {
		log.Printf("[MinIO] deleted: %s", objName)
		return true
	}

	log.Printf("[MinIO] delete returned %s (file: %s)", status, objName)
	return false
}
