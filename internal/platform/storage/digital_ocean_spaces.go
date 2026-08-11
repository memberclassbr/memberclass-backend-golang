package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	"github.com/memberclass-backend-golang/internal/platform/config"
	"github.com/memberclass-backend-golang/internal/platform/logger"
	"github.com/memberclass-backend-golang/internal/platform/telemetry"
)

type DigitalOceanSpaces struct {
	client    *s3.Client
	bucket    string
	region    string
	endpoint  string
	publicURL string
	logger    logger.Logger
}

func NewDigitalOceanSpaces(appCfg *config.Config, logger logger.Logger) (Storage, error) {
	accessKey := appCfg.Storage.AccessKey
	secretKey := appCfg.Storage.SecretKey
	// Every write goes to this one bucket. Reads still resolve the bucket
	// from the object URL, so media uploaded before this deployment owned
	// its own bucket stays readable.
	bucket := appCfg.Storage.Bucket
	spacesURL := appCfg.Storage.URL

	region := extractRegionFromURL(spacesURL)
	endpoint := spacesURL
	publicURL := fmt.Sprintf("https://%s.%s.digitaloceanspaces.com", bucket, region)
	// The SDK gets an instrumented HTTP client rather than its own, so uploads
	// to Spaces appear as client spans on the request that triggered them.
	// No Timeout on purpose: the SDK bounds its own operations, and a blanket
	// deadline here would cut large video uploads short.
	spacesHTTP := &http.Client{Transport: telemetry.Transport(nil)}

	cfg, err := awsconfig.LoadDefaultConfig(context.TODO(),
		awsconfig.WithRegion(region),
		awsconfig.WithHTTPClient(spacesHTTP),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")),
		awsconfig.WithEndpointResolverWithOptions(aws.EndpointResolverWithOptionsFunc(
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
	return publicURL, nil
}

func (d *DigitalOceanSpaces) Download(ctx context.Context, urlOrKey string) ([]byte, error) {
	key := d.extractKeyFromURL(urlOrKey)
	bucket := d.extractBucketFromURL(urlOrKey)

	d.logger.Info("Downloading file from DigitalOcean Spaces", "key", key, "bucket", bucket)

	input := &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	}

	result, err := d.client.GetObject(ctx, input)
	if err != nil {
		d.logger.Error("Failed to download file from DigitalOcean Spaces", "key", key, "bucket", bucket, "error", err)
		return nil, fmt.Errorf("failed to download file: %w", err)
	}
	defer result.Body.Close()

	data, err := io.ReadAll(result.Body)
	if err != nil {
		d.logger.Error("Failed to read downloaded file", "key", key, "error", err)
		return nil, fmt.Errorf("failed to read file content: %w", err)
	}

	d.logger.Info("File downloaded successfully", "key", key, "bucket", bucket, "size", len(data))
	return data, nil
}

func (d *DigitalOceanSpaces) Delete(ctx context.Context, urlOrKey string) error {
	key := d.extractKeyFromURL(urlOrKey)
	bucket := d.extractBucketFromURL(urlOrKey)

	d.logger.Info("Deleting file from DigitalOcean Spaces", "key", key, "bucket", bucket)

	input := &s3.DeleteObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	}

	_, err := d.client.DeleteObject(ctx, input)
	if err != nil {
		d.logger.Error("Failed to delete file from DigitalOcean Spaces", "key", key, "bucket", bucket, "error", err)
		return fmt.Errorf("failed to delete file: %w", err)
	}

	d.logger.Info("File deleted successfully", "key", key, "bucket", bucket)
	return nil
}

func (d *DigitalOceanSpaces) Exists(ctx context.Context, urlOrKey string) (bool, error) {
	key := d.extractKeyFromURL(urlOrKey)
	bucket := d.extractBucketFromURL(urlOrKey)

	d.logger.Debug("Checking if file exists in DigitalOcean Spaces", "key", key, "bucket", bucket)

	input := &s3.HeadObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	}

	_, err := d.client.HeadObject(ctx, input)
	if err != nil {
		if !strings.Contains(err.Error(), "NotFound") {
			d.logger.Error("Error checking file existence", "key", key, "bucket", bucket, "error", err)
			return false, fmt.Errorf("failed to check file existence: %w", err)
		}
		return false, nil
	}

	d.logger.Debug("File exists", "key", key, "bucket", bucket)
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

func (d *DigitalOceanSpaces) extractBucketFromURL(urlOrKey string) string {
	if !strings.HasPrefix(urlOrKey, "http") {
		return d.bucket
	}

	parsedURL, err := url.Parse(urlOrKey)
	if err != nil {
		return d.bucket
	}

	host := parsedURL.Hostname()
	parts := strings.SplitN(host, ".", 2)
	if len(parts) < 2 {
		return d.bucket
	}

	return parts[0]
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
