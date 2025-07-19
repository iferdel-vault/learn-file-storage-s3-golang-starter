package main

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func (cfg apiConfig) ensureAssetsDir() error {
	if _, err := os.Stat(cfg.assetsRoot); os.IsNotExist(err) {
		return os.Mkdir(cfg.assetsRoot, 0755)
	}
	return nil
}

func getAssetPath(mediaType string) string {
	fileExtension := mediaTypeToExt(mediaType)
	b := make([]byte, 32)
	rand.Read(b)
	fileName := hex.EncodeToString(b)
	filePath := fmt.Sprintf("%s.%s", fileName, fileExtension)
	return filePath
}

func mediaTypeToExt(mediaType string) string {
	parts := strings.Split(mediaType, "/")
	if len(parts) != 2 {
		return "bin"
	}
	return parts[1]
}

func (cfg apiConfig) getObjectURL(key string) string {
	return fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", cfg.s3Bucket, cfg.s3Region, key)
}

func GCD(a, b int) int {
	for b != 0 {
		t := b
		b = a % b
		a = t
	}
	return a
}

func getVideoAspectRatio(filePath string) (string, error) {
	cmd := exec.Command("ffprobe", "-v error", "print_format json", fmt.Sprintf("-show_streams %s", filePath))
	var buffer bytes.Buffer
	cmd.Stdout = &buffer
	err := cmd.Run()
	if err != nil {
		return "", err
	}

	type videoDims struct {
		Width  int `json:"width"`
		Height int `json:"height"`
	}
	d := videoDims{}
	err = json.Unmarshal(buffer.Bytes(), &d)
	if err != nil {
		return "", fmt.Errorf("error on unmarshalling video dimentions from output of ffprobe cmd: %s", err)
	}

	gcd := GCD(d.Width, d.Height)
	aspectRatio := fmt.Sprintf("%d:%d", d.Width/gcd, d.Height/gcd)

	return aspectRatio, nil
}
