package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/database"
)

func (cfg *apiConfig) ensureAssetsDir() error {
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

func getVideoAspectRatio(filePath string) (string, error) {
	args := []string{
		"-v", "error",
		"-print_format", "json",
		"-show_streams",
		filePath,
	}
	cmd := exec.Command("ffprobe", args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("ffprobe error: %v", err)
	}
	if stderr.String() != "" {
		return "", errors.New(stderr.String())
	}

	type output struct {
		Streams []struct {
			Width  int `json:"width"`
			Height int `json:"height"`
		} `json:"streams"`
	}
	d := output{}
	err := json.Unmarshal(stdout.Bytes(), &d)
	if err != nil {
		return "", fmt.Errorf("error on unmarshalling video dimentions from output of ffprobe cmd: %s", err)
	}
	if len(d.Streams) == 0 {
		return "", errors.New("no video streams found")
	}

	width, height := d.Streams[0].Width, d.Streams[0].Height
	if height == 0 {
		return "", fmt.Errorf("invalid height value: 0")
	}

	ratio := float64(width) / float64(height)
	const aspectRatioTolerance = 0.01
	aspectRatio, err := getAspectRatio(ratio, aspectRatioTolerance)
	if err != nil {
		return "", err
	}
	return aspectRatio, nil
}

func getAspectRatio(ratio, tolerance float64) (string, error) {
	switch {
	case nearlyEqual(ratio, 16.0/9.0, tolerance):
		return "16:9", nil
	case nearlyEqual(ratio, 9.0/16.0, tolerance):
		return "9:16", nil
	default:
		return fmt.Sprintf("%.0f", ratio), nil
	}
}

func nearlyEqual(a, b, tolerance float64) bool {
	return math.Abs(a-b) <= tolerance
}

func setVideoSchemaPrefix(key, aspectRatio string) string {
	switch aspectRatio {
	case "16:9":
		return fmt.Sprintf("landscape/%s", key)
	case "9:16":
		return fmt.Sprintf("portrait/%s", key)
	default:
		return fmt.Sprintf("other/%s", key)
	}
}

// return a new filepath of the same file but encoded with fast start...
func processVideoForFastStart(filePath string) (string, error) {
	outputFilePath := filePath + ".processing"
	cmd := exec.Command("ffmpeg", "-i", filePath, "-c", "copy", "-movflags", "faststart", "-f", "mp4", outputFilePath)
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("ffmpeg error: %v", err)
	}
	return outputFilePath, nil
}

func generatePresignedURL(s3Client *s3.Client, bucket, key string, expireTime time.Duration) (string, error) {
	client := s3.NewPresignClient(s3Client)
	r, err := client.PresignGetObject(context.Background(), &s3.GetObjectInput{}, s3.WithPresignExpires(expireTime))
	if err != nil {
		return "", err
	}
	return r.URL, nil
}

func (cfg *apiConfig) dbVideoToSignedVideo(video database.Video) (database.Video, error) {
	s := strings.Split(*video.VideoURL, ",")
	bucket, key := s[0], s[1]

	url, err := generatePresignedURL(
		cfg.s3Client,
		bucket,
		key,
		time.Hour*1,
	)
	if err != nil {
		return database.Video{}, err
	}
	video.VideoURL = &url
	return video, nil
}
