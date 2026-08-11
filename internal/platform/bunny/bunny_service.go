package bunny

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/memberclass-backend-golang/internal/platform/config"
	"github.com/memberclass-backend-golang/internal/platform/logger"
)

type BunnyService struct {
	client  *http.Client
	baseURL string
	log     logger.Logger
}

func (b *BunnyService) CreateCollection(ctx context.Context, createCollectionRequest CreateCollectionRequest, bunnyParametersAccess ParametersAccess) (*CreateCollectionResponse, error) {
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

	var collection CreateCollectionResponse
	if err := json.NewDecoder(resp.Body).Decode(&collection); err != nil {
		b.log.Error("Failed to decode response", "error", err, "url", url)
		return nil, err
	}

	b.log.Info("Collection created successfully", "name", createCollectionRequest.Name, "guid", collection.GUID)
	return &collection, nil

}

func (b *BunnyService) CreateVideo(ctx context.Context, video CreateVideoRequest, bunnyParametersAccess ParametersAccess) (*CreateVideoResponse, error) {
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

	var result CreateVideoResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		b.log.Error("Failed to decode response", "error", err, "url", url)
		return nil, err
	}

	b.log.Info("Video created successfully", "title", video.Title, "guid", result.GUID)
	return &result, nil
}

func (b *BunnyService) UploadVideo(ctx context.Context, uploadVideoRequest UploadVideoRequest, bunnyParametersAccess ParametersAccess) error {
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

func (b *BunnyService) GetCollections(ctx context.Context, bunnyParametersAccess ParametersAccess) (*CollectionsResponse, error) {

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

	var collectionsResponse CollectionsResponse
	err = json.NewDecoder(resp.Body).Decode(&collectionsResponse)
	if err != nil {
		b.log.Error("Failed to decode response", "error", err, "url", url)
		return nil, err
	}

	b.log.Debug("Collections retrieved successfully", "count", len(collectionsResponse.Items))
	return &collectionsResponse, nil

}

// NewBunnyService builds the client from the validated config. The timeout and
// base URL used to be parsed from the environment on every construction, with
// an unparsable timeout silently falling back to 30s; config resolves both once
// at startup.
func NewBunnyService(cfg *config.Config, log logger.Logger) Service {
	log.Info("BunnyService initialized", "timeout", cfg.Bunny.Timeout.String())

	return &BunnyService{
		client:  &http.Client{Timeout: cfg.Bunny.Timeout},
		baseURL: cfg.Bunny.BaseURL,
		log:     log,
	}
}
