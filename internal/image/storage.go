package image

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (l ImageLocalizer) downloadAndSave(ctx context.Context, imageURL string) (string, error) {
	parsedReq, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
	if err != nil {
		return "", err
	}
	if err := validateRemoteURL(parsedReq.URL); err != nil {
		return "", err
	}
	resp, err := l.Client.Do(parsedReq)
	if err != nil {
		return "", err
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			_ = closeErr
		}
	}()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	contentType := strings.ToLower(strings.TrimSpace(strings.Split(resp.Header.Get("Content-Type"), ";")[0]))
	if !strings.HasPrefix(contentType, "image/") {
		return "", fmt.Errorf("Content-Type is %s, not image/*", contentType)
	}

	limited := io.LimitReader(resp.Body, l.MaxBytes+1)
	tmp, err := os.CreateTemp("", "image-localizer-*")
	if err != nil {
		return "", err
	}
	tmpName := tmp.Name()
	defer func() {
		if removeErr := os.Remove(tmpName); removeErr != nil && !os.IsNotExist(removeErr) {
			_ = removeErr
		}
	}()

	hasher := sha256.New()
	header := make([]byte, 0, 512)
	var written int64
	buf := make([]byte, 32*1024)
	for {
		n, readErr := limited.Read(buf)
		if n > 0 {
			written += int64(n)
			if written > l.MaxBytes {
				if closeErr := tmp.Close(); closeErr != nil {
					return "", closeErr
				}
				return "", fmt.Errorf("image exceeds %d bytes", l.MaxBytes)
			}
			chunk := buf[:n]
			if len(header) < 512 {
				need := 512 - len(header)
				if need > len(chunk) {
					need = len(chunk)
				}
				header = append(header, chunk[:need]...)
			}
			if _, err := hasher.Write(chunk); err != nil {
				return "", err
			}
			if _, err := tmp.Write(chunk); err != nil {
				return "", err
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return "", readErr
		}
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}

	ext, err := imageExtFromMagic(header)
	if err != nil {
		return "", err
	}
	sum := hex.EncodeToString(hasher.Sum(nil))
	now := time.Now()
	relDir := filepath.Join(now.Format("2006"), now.Format("01"))
	targetDir := filepath.Join(l.StorageDir, relDir)
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return "", err
	}
	filename := sum + ext
	targetPath := filepath.Join(targetDir, filename)
	if info, err := os.Stat(targetPath); err == nil && info.Size() == written {
		return l.publicPath(relDir, filename), nil
	}

	staged := filepath.Join(targetDir, "."+filename+"."+randomSuffix()+".tmp")
	if err := os.Rename(tmpName, staged); err != nil {
		return "", err
	}
	tmpName = staged
	defer func() {
		if removeErr := os.Remove(staged); removeErr != nil && !os.IsNotExist(removeErr) {
			_ = removeErr
		}
	}()
	if err := os.Rename(staged, targetPath); err != nil {
		if os.IsExist(err) {
			return l.publicPath(relDir, filename), nil
		}
		if info, statErr := os.Stat(targetPath); statErr == nil && info.Size() == written {
			return l.publicPath(relDir, filename), nil
		}
		return "", err
	}
	return l.publicPath(relDir, filename), nil
}

func (l ImageLocalizer) publicPath(relDir, filename string) string {
	return strings.TrimRight(l.PublicPrefix, "/") + "/" + filepath.ToSlash(filepath.Join(relDir, filename))
}

func imageExtFromMagic(data []byte) (string, error) {
	if len(data) >= 3 && data[0] == 0xff && data[1] == 0xd8 && data[2] == 0xff {
		return ".jpg", nil
	}
	if len(data) >= 8 && data[0] == 0x89 && data[1] == 0x50 && data[2] == 0x4e && data[3] == 0x47 && data[4] == 0x0d && data[5] == 0x0a && data[6] == 0x1a && data[7] == 0x0a {
		return ".png", nil
	}
	if len(data) >= 6 && (string(data[:6]) == "GIF87a" || string(data[:6]) == "GIF89a") {
		return ".gif", nil
	}
	if len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP" {
		return ".webp", nil
	}
	return "", fmt.Errorf("unsupported image type")
}

func randomSuffix() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}
