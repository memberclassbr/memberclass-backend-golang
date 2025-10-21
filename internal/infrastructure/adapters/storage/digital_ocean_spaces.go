package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/memberclass-backend-golang/internal/domain/ports"
)

type DigitalOceanSpaces struct {
	client    *s3.Client
	bucket    string
	region    string
	endpoint  string
	publicURL string
	logger    ports.Logger
}

func NewDigitalOceanSpaces(logger ports.Logger) (ports.Storage, error) {
	accessKey := os.Getenv("DO_SPACES_ID")
	secretKey := os.Getenv("DO_SPACES_SECRET")
	bucket := os.Getenv("DO_SPACES_BUCKET")
	spacesURL := os.Getenv("DO_SPACES_URL")

	if accessKey == "" || secretKey == "" || bucket == "" || spacesURL == "" {
		return nil, fmt.Errorf("missing required environment variables: DO_SPACES_ID, DO_SPACES_SECRET, DO_SPACES_BUCKET, DO_SPACES_URL")
	}

	region := extractRegionFromURL(spacesURL)
	endpoint := spacesURL
	publicURL := fmt.Sprintf("https://%s.%s.digitaloceanspaces.com", bucket, region)
	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithRegion(region),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")),
		config.WithEndpointResolverWithOptions(aws.EndpointResolverWithOptionsFunc(
			func(service, region string, options ...interface{}) (aws.Endpoint, error) {
				return aws.Endpoint{
					URL:           endpoint,
					SigningRegion: region,
				}, nil
			})),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.UsePathStyle = true
	})

	return &DigitalOceanSpaces{
		client:    client,
		bucket:    bucket,
		region:    region,
		endpoint:  endpoint,
		publicURL: publicURL,
		logger:    logger,
	}, nil
}

func (d *DigitalOceanSpaces) Upload(ctx context.Context, data []byte, filename string, contentType string) (string, error) {
	d.logger.Info("Uploading file to DigitalOcean Spaces", "filename", filename, "size", len(data))

	input := &s3.PutObjectInput{
		Bucket:      aws.String(d.bucket),
		Key:         aws.String(filename),
		Body:        bytes.NewReader(data),
		ContentType: aws.String(contentType),
		ACL:         types.ObjectCannedACLPublicRead,
	}
	_, err := d.client.PutObject(ctx, input)
	if err != nil {
		d.logger.Error("Failed to upload file to DigitalOcean Spaces", "filename", filename, "error", err)
		return "", fmt.Errorf("failed to upload file: %w", err)
	}

	publicURL := fmt.Sprintf("%s/%s", d.publicURL, filename)
	d.logger.Info("File uploaded successfully", "filename", filename, "url", publicURL)

	return publicURL, nil
}

func (d *DigitalOceanSpaces) Download(ctx context.Context, urlOrKey string) ([]byte, error) {
	key := d.extractKeyFromURL(urlOrKey)

	d.logger.Info("Downloading file from DigitalOcean Spaces", "key", key)

	input := &s3.GetObjectInput{
		Bucket: aws.String(d.bucket),
		Key:    aws.String(key),
	}

	result, err := d.client.GetObject(ctx, input)
	if err != nil {
		d.logger.Error("Failed to download file from DigitalOcean Spaces", "key", key, "error", err)
		return nil, fmt.Errorf("failed to download file: %w", err)
	}
	defer result.Body.Close()

	data, err := io.ReadAll(result.Body)
	if err != nil {
		d.logger.Error("Failed to read downloaded file", "key", key, "error", err)
		return nil, fmt.Errorf("failed to read file content: %w", err)
	}

	d.logger.Info("File downloaded successfully", "key", key, "size", len(data))
	return data, nil
}

func (d *DigitalOceanSpaces) Delete(ctx context.Context, urlOrKey string) error {
	key := d.extractKeyFromURL(urlOrKey)

	d.logger.Info("Deleting file from DigitalOcean Spaces", "key", key)

	input := &s3.DeleteObjectInput{
		Bucket: aws.String(d.bucket),
		Key:    aws.String(key),
	}

	_, err := d.client.DeleteObject(ctx, input)
	if err != nil {
		d.logger.Error("Failed to delete file from DigitalOcean Spaces", "key", key, "error", err)
		return fmt.Errorf("failed to delete file: %w", err)
	}

	d.logger.Info("File deleted successfully", "key", key)
	return nil
}

func (d *DigitalOceanSpaces) Exists(ctx context.Context, urlOrKey string) (bool, error) {
	key := d.extractKeyFromURL(urlOrKey)

	d.logger.Debug("Checking if file exists in DigitalOcean Spaces", "key", key)

	input := &s3.HeadObjectInput{
		Bucket: aws.String(d.bucket),
		Key:    aws.String(key),
	}

	_, err := d.client.HeadObject(ctx, input)
	if err != nil {
		if err != nil && !strings.Contains(err.Error(), "NotFound") {
			d.logger.Error("Error checking file existence", "key", key, "error", err)
			return false, fmt.Errorf("failed to check file existence: %w", err)
		}
		return false, nil
	}

	d.logger.Debug("File exists", "key", key)
	return true, nil
}

func (d *DigitalOceanSpaces) extractKeyFromURL(urlOrKey string) string {
	if strings.HasPrefix(urlOrKey, "http") {
		parsedURL, err := url.Parse(urlOrKey)
		if err != nil {
			return urlOrKey
		}
		key := strings.TrimPrefix(parsedURL.Path, "/")
		return key
	}
	return urlOrKey
}

func (d *DigitalOceanSpaces) GetPublicURL(key string) string {
	return fmt.Sprintf("%s/%s", d.publicURL, key)
}

func extractRegionFromURL(url string) string {
	parts := strings.Split(url, "://")
	if len(parts) != 2 {
		return "nyc3"
	}

	host := parts[1]
	regionParts := strings.Split(host, ".")
	if len(regionParts) > 0 {
		return regionParts[0]
	}

	return "nyc3"
}
