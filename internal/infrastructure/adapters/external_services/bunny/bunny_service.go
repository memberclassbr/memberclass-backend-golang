package bunny

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/memberclass-backend-golang/internal/domain/dto"
	"github.com/memberclass-backend-golang/internal/domain/ports"
	"github.com/memberclass-backend-golang/internal/domain/ports/bunny"
)

type BunnyService struct {
	client  *http.Client
	baseURL string
	log     ports.Logger
}

func (b *BunnyService) CreateCollection(ctx context.Context, createCollectionRequest dto.CreateCollectionRequest, bunnyParametersAccess dto.BunnyParametersAccess) (*dto.CreateCollectionResponse, error) {
	if bunnyParametersAccess.LibraryID == "" || bunnyParametersAccess.LibraryApiKey == "" {
		return nil, errors.New("libraryID and libraryApiKey is required")
	}

	var builder strings.Builder
	builder.WriteString(b.baseURL)
	builder.WriteString(bunnyParametersAccess.LibraryID)
	builder.WriteString("/collections")
	url := builder.String()

	b.log.Debug("Creating collection in Bunny", "name", createCollectionRequest.Name, "url", url)

	header := http.Header{
		"Content-Type": []string{"application/json"},
		"AccessKey":    []string{bunnyParametersAccess.LibraryApiKey},
	}

	reqBody, err := json.Marshal(createCollectionRequest)
	if err != nil {
		b.log.Error("Failed to marshal request body", "error", err, "request", createCollectionRequest)
		return nil, err
	}

	r, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(reqBody))
	if err != nil {
		b.log.Error("Failed to create HTTP request", "error", err, "url", url)
		return nil, err
	}

	r.Header = header

	resp, err := b.client.Do(r)
	if err != nil {
		b.log.Error("HTTP request failed", "error", err, "url", url)
		return nil, err
	}
	defer resp.Body.Close()

	b.log.Debug("HTTP response received", "statusCode", resp.StatusCode, "url", url)

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		b.log.Error("Bunny API returned error", "statusCode", resp.StatusCode, "status", resp.Status, "url", url, "responseBody", string(bodyBytes))
		return nil, errors.New(resp.Status)
	}

	var collection dto.CreateCollectionResponse
	if err := json.NewDecoder(resp.Body).Decode(&collection); err != nil {
		b.log.Error("Failed to decode response", "error", err, "url", url)
		return nil, err
	}

	b.log.Info("Collection created successfully", "name", createCollectionRequest.Name, "guid", collection.GUID)
	return &collection, nil

}

func (b *BunnyService) CreateVideo(ctx context.Context, video dto.CreateVideoRequest, bunnyParametersAccess dto.BunnyParametersAccess) (*dto.CreateVideoResponse, error) {
	if video.Title == "" || video.CollectionID == "" {
		return nil, errors.New("title and collectionID are required")
	}

	var builder strings.Builder
	builder.WriteString(b.baseURL)
	builder.WriteString(bunnyParametersAccess.LibraryID)
	builder.WriteString("/videos")
	url := builder.String()

	b.log.Debug("Creating video in Bunny", "title", video.Title, "collectionID", video.CollectionID, "url", url)

	header := http.Header{
		"Content-Type": []string{"application/json"},
		"AccessKey":    []string{bunnyParametersAccess.LibraryApiKey},
	}

	reqBody, err := json.Marshal(video)
	if err != nil {
		b.log.Error("Failed to marshal request body", "error", err, "request", video)
		return nil, err
	}

	r, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(reqBody))
	if err != nil {
		b.log.Error("Failed to create HTTP request", "error", err, "url", url)
		return nil, err
	}

	r.Header = header

	resp, err := b.client.Do(r)
	if err != nil {
		b.log.Error("HTTP request failed", "error", err, "url", url)
		return nil, err
	}
	defer resp.Body.Close()

	b.log.Debug("HTTP response received", "statusCode", resp.StatusCode, "url", url)

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		b.log.Error("Bunny API returned error", "status", resp.Status, "url", url, "responseBody", string(bodyBytes))
		return nil, errors.New(resp.Status)
	}

	var result dto.CreateVideoResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		b.log.Error("Failed to decode response", "error", err, "url", url)
		return nil, err
	}

	b.log.Info("Video created successfully", "title", video.Title, "guid", result.GUID)
	return &result, nil
}

func (b *BunnyService) UploadVideo(ctx context.Context, uploadVideoRequest dto.UploadVideoRequest, bunnyParametersAccess dto.BunnyParametersAccess) error {
	if bunnyParametersAccess.LibraryID == "" || bunnyParametersAccess.LibraryApiKey == "" {
		return errors.New("libraryID and libraryApiKey is required")
	}

	var builder strings.Builder
	builder.WriteString(b.baseURL)
	builder.WriteString(bunnyParametersAccess.LibraryID)
	builder.WriteString("/videos/")
	builder.WriteString(uploadVideoRequest.GUID)
	url := builder.String()

	b.log.Debug("Uploading video file to Bunny", "guid", uploadVideoRequest.GUID, "fileSize", len(uploadVideoRequest.File), "contentType", uploadVideoRequest.ContentType, "url", url)

	header := http.Header{
		"Content-Type": []string{uploadVideoRequest.ContentType},
		"AccessKey":    []string{bunnyParametersAccess.LibraryApiKey},
	}
	r, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(uploadVideoRequest.File))
	if err != nil {
		b.log.Error("Failed to create HTTP request", "error", err, "url", url)
		return err
	}

	r.Header = header

	resp, err := b.client.Do(r)
	if err != nil {
		b.log.Error("HTTP request failed", "error", err, "url", url)
		return err
	}

	defer resp.Body.Close()

	b.log.Debug("HTTP response received", "statusCode", resp.StatusCode, "url", url)

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		b.log.Error("Bunny API returned error", "statusCode", resp.StatusCode, "status", resp.Status, "url", url, "responseBody", string(bodyBytes))
		return errors.New(resp.Status)
	}

	b.log.Info("Video file uploaded successfully", "guid", uploadVideoRequest.GUID, "fileSize", len(uploadVideoRequest.File))
	return nil
}

func (b *BunnyService) GetCollections(ctx context.Context, bunnyParametersAccess dto.BunnyParametersAccess) (*dto.BunnyCollectionsResponse, error) {

	if bunnyParametersAccess.LibraryID == "" || bunnyParametersAccess.LibraryApiKey == "" {
		return nil, errors.New("libraryID and libraryApiKey is required")
	}

	var builder strings.Builder
	builder.WriteString(b.baseURL)
	builder.WriteString(bunnyParametersAccess.LibraryID)
	builder.WriteString("/collections?libraryId=")
	builder.WriteString(bunnyParametersAccess.LibraryID)
	url := builder.String()

	b.log.Debug("Getting collections from Bunny", "url", url)

	header := http.Header{
		"Content-Type": []string{"application/json"},
		"AccessKey":    []string{bunnyParametersAccess.LibraryApiKey},
	}

	r, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		b.log.Error("Failed to create HTTP request", "error", err, "url", url)
		return nil, err
	}

	r.Header = header

	resp, err := b.client.Do(r)
	if err != nil {
		b.log.Error("HTTP request failed", "error", err, "url", url)
		return nil, err
	}

	defer resp.Body.Close()

	b.log.Debug("HTTP response received", "statusCode", resp.StatusCode, "url", url)

	var collectionsResponse dto.BunnyCollectionsResponse
	err = json.NewDecoder(resp.Body).Decode(&collectionsResponse)
	if err != nil {
		b.log.Error("Failed to decode response", "error", err, "url", url)
		return nil, err
	}

	b.log.Debug("Collections retrieved successfully", "count", len(collectionsResponse.Items))
	return &collectionsResponse, nil

}

func NewBunnyService(log ports.Logger) bunny.BunnyService {
	timeoutStr := os.Getenv("BUNNY_TIMEOUT_SECONDS")
	timeout := 30 * time.Second

	if timeoutStr != "" {
		if parsedTimeout, err := time.ParseDuration(timeoutStr + "s"); err == nil {
			timeout = parsedTimeout
		} else {
			log.Warn("Invalid BUNNY_TIMEOUT_SECONDS, using default", "value", timeoutStr, "default", "30s")
		}
	}

	log.Info("BunnyService initialized", "timeout", timeout.String())

	return &BunnyService{
		client: &http.Client{
			Timeout: timeout,
		},
		baseURL: os.Getenv("BUNNY_BASE_URL"),
		log:     log,
	}
}
