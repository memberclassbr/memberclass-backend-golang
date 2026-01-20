package ilovepdf

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/memberclass-backend-golang/internal/domain/dto"
	"github.com/memberclass-backend-golang/internal/domain/ports"
	"github.com/memberclass-backend-golang/internal/domain/ports/pdf_processor"
)

type IlovePdfService struct {
	apiKeys []dto.ApiKeyInfo
	baseURL string
	log     ports.Logger
	cache   ports.Cache
	mutex   sync.RWMutex
}

func NewIlovePdfService(log ports.Logger, cache ports.Cache) (pdf_processor.PdfProcessService, error) {
	service := &IlovePdfService{
		log:   log,
		cache: cache,
	}

	if err := service.loadApiKeys(); err != nil {
		return nil, fmt.Errorf("failed to load API keys: %w", err)
	}

	baseURL := os.Getenv("ILOVEPDF_BASE_URL")
	if baseURL == "" {
		baseURL = "https://api.ilovepdf.com/v1"
	}
	service.baseURL = baseURL

	return service, nil
}

func (i *IlovePdfService) loadApiKeys() error {
	keysEnv := os.Getenv("ILOVEPDF_API_KEYS")
	if keysEnv == "" {
		return errors.New("ILOVEPDF_API_KEYS environment variable not set")
	}

	keyStrings := strings.Split(keysEnv, ",")
	i.apiKeys = make([]dto.ApiKeyInfo, 0, len(keyStrings))

	for _, keyStr := range keyStrings {
		keyStr = strings.TrimSpace(keyStr)
		if keyStr == "" {
			continue
		}

		keyInfo := dto.ApiKeyInfo{
			Key:      keyStr,
			LastUsed: "",
		}

		i.apiKeys = append(i.apiKeys, keyInfo)
	}

	if len(i.apiKeys) == 0 {
		return errors.New("no valid API keys found")
	}

	return nil
}

func (i *IlovePdfService) getActiveKey() (string, error) {
	i.mutex.Lock()
	defer i.mutex.Unlock()

	ctx := context.Background()

	for _, keyInfo := range i.apiKeys {
		blacklistKey := fmt.Sprintf("ilovepdf_blacklist:%s", keyInfo.Key)

		exists, err := i.cache.Exists(ctx, blacklistKey)
		if err != nil {
			i.log.Warn(fmt.Sprintf("Failed to check blacklist for key: %v", err))
			continue
		}

		if !exists {
			keyInfo.LastUsed = time.Now().Format(time.RFC3339)
			return keyInfo.Key, nil
		}
	}

	return "", errors.New("all API keys are blacklisted (credits exhausted)")
}

func (i *IlovePdfService) blacklistKey(key string) error {
	blacklistKey := fmt.Sprintf("ilovepdf_blacklist:%s", key)
	ttl := 30 * 24 * time.Hour // 30 dias
	ctx := context.Background()

	err := i.cache.Set(ctx, blacklistKey, "exhausted", ttl)
	if err != nil {
		return fmt.Errorf("failed to blacklist key: %w", err)
	}

	// Log only first 20 characters safely
	keyPreview := key
	if len(key) > 20 {
		keyPreview = key[:20]
	}
	i.log.Warn(fmt.Sprintf("Blacklisted API key (credits exhausted): %s...", keyPreview))
	return nil
}

func (i *IlovePdfService) isCreditsExhaustedError(err error) bool {
	if err == nil {
		return false
	}

	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "credits") ||
		strings.Contains(errStr, "limit") ||
		strings.Contains(errStr, "exhausted") ||
		strings.Contains(errStr, "quota")
}

// GetToken - Get authentication token
func (i *IlovePdfService) GetToken() (string, error) {
	activeKey, err := i.getActiveKey()
	if err != nil {
		return "", fmt.Errorf("failed to get active API key: %w", err)
	}

	authURL := fmt.Sprintf("%s/auth", i.baseURL)

	payload := map[string]string{
		"public_key": activeKey,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal auth payload: %w", err)
	}

	resp, err := http.Post(authURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to authenticate: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		errorMsg := fmt.Sprintf("authentication failed: %s - %s", resp.Status, string(body))

		// Check if it's a credits exhausted error
		if i.isCreditsExhaustedError(fmt.Errorf("%s", errorMsg)) {
			if blacklistErr := i.blacklistKey(activeKey); blacklistErr != nil {
				i.log.Warn(fmt.Sprintf("Failed to blacklist key: %v", blacklistErr))
			}
		}

		return "", fmt.Errorf("%s", errorMsg)
	}

	var result dto.AuthResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode auth response: %w", err)
	}

	return result.Token, nil
}

// CreateTask - Create new task
func (i *IlovePdfService) CreateTask(token string) (*dto.TaskResponse, error) {
	taskURL := fmt.Sprintf("%s/start/pdfjpg/us", i.baseURL)

	req, err := http.NewRequest("GET", taskURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create task request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to create task: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("create task failed: %s - %s", resp.Status, string(body))
	}

	var result dto.TaskResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode task response: %w", err)
	}

	return &result, nil
}

// AddFile - Upload PDF file to iLovePDF server
func (i *IlovePdfService) AddFile(token, taskID, pdfURL, server string) (string, error) {
	// 1. Download PDF from URL
	pdfResponse, err := http.Get(pdfURL)
	if err != nil {
		return "", fmt.Errorf("failed to download PDF: %w", err)
	}
	defer pdfResponse.Body.Close()

	if pdfResponse.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to download PDF: %s", pdfResponse.Status)
	}

	// 2. Read PDF content
	pdfData, err := io.ReadAll(pdfResponse.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read PDF data: %w", err)
	}

	// 3. Create form data for upload
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// Add task field
	taskField, err := writer.CreateFormField("task")
	if err != nil {
		return "", fmt.Errorf("failed to create task field: %w", err)
	}
	taskField.Write([]byte(taskID))

	// Add PDF file
	fileField, err := writer.CreateFormFile("file", "document.pdf")
	if err != nil {
		return "", fmt.Errorf("failed to create file field: %w", err)
	}
	fileField.Write(pdfData)

	writer.Close()

	// 4. Make POST request to upload
	uploadURL := fmt.Sprintf("https://%s/v1/upload", server)
	req, err := http.NewRequest("POST", uploadURL, &buf)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// Headers - FIXED: use token instead of publicKey
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	// 5. Execute request
	client := &http.Client{Timeout: 60 * time.Second} // Longer timeout for uploads
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to upload file: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("failed to add file: %s - %s", resp.Status, string(body))
	}

	var result dto.UploadResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	return result.ServerFilename, nil
}

// ProcessTask - Process PDF to images
func (i *IlovePdfService) ProcessTask(token, taskID, serverFilename, server string) error {
	processURL := fmt.Sprintf("https://%s/v1/process", server)

	payload := map[string]interface{}{
		"task": taskID,
		"tool": "pdfjpg",
		"files": []map[string]string{
			{
				"server_filename": serverFilename,
				"filename":        "document.pdf",
			},
		},
		"ignore_errors":  true,
		"try_pdf_repair": true,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal process payload: %w", err)
	}

	req, err := http.NewRequest("POST", processURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create process request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 120 * time.Second} // Longer timeout for processing
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to process task: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("process task failed: %s - %s", resp.Status, string(body))
	}

	var result dto.ProcessResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("failed to decode process response: %w", err)
	}

	if result.Status != "TaskSuccess" {
		return fmt.Errorf("task failed: %s", result.Error)
	}

	return nil
}

// DownloadTask - Download ZIP with images
func (i *IlovePdfService) DownloadTask(token, taskID, server string) ([]byte, error) {
	downloadURL := fmt.Sprintf("https://%s/v1/download/%s", server, taskID)

	req, err := http.NewRequest("GET", downloadURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create download request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to download task: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("download failed: %s - %s", resp.Status, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	return body, nil
}

// ExtractImagesFromZip - Extract images from ZIP file or single JPEG
func (i *IlovePdfService) ExtractImagesFromZip(zipData []byte) ([]string, error) {
	if len(zipData) < 4 {
		return nil, fmt.Errorf("data too short to be a valid file (%d bytes)", len(zipData))
	}

	// Check if it's a JPEG image (starts with JPEG signature)
	if i.isJPEGImage(zipData) {
		return i.handleSingleJPEG(zipData)
	}

	// Check if it's a ZIP file (starts with ZIP signature)
	if i.isZIPFile(zipData) {
		return i.handleZIPFile(zipData)
	}

	return nil, fmt.Errorf("data is neither a valid JPEG image nor ZIP file")
}

// isJPEGImage - Check if data is a JPEG image
func (i *IlovePdfService) isJPEGImage(data []byte) bool {
	// JPEG files start with 0xFF 0xD8
	if len(data) < 2 {
		return false
	}
	return data[0] == 0xFF && data[1] == 0xD8
}

// isZIPFile - Check if data is a ZIP file
func (i *IlovePdfService) isZIPFile(data []byte) bool {
	// ZIP files start with "PK" (0x504B)
	if len(data) < 2 {
		return false
	}
	return data[0] == 0x50 && data[1] == 0x4B
}

// handleSingleJPEG - Handle single JPEG image
func (i *IlovePdfService) handleSingleJPEG(jpegData []byte) ([]string, error) {
	// Convert to base64
	base64Data := fmt.Sprintf("data:image/jpeg;base64,%s",
		base64.StdEncoding.EncodeToString(jpegData))

	images := []string{base64Data}
	return images, nil
}

// handleZIPFile - Handle ZIP file with multiple images
func (i *IlovePdfService) handleZIPFile(zipData []byte) ([]string, error) {
	reader, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		return nil, fmt.Errorf("failed to read zip: %w", err)
	}

	var images []string
	for _, file := range reader.File {
		// Filter only image files
		if strings.HasSuffix(strings.ToLower(file.Name), ".jpg") ||
			strings.HasSuffix(strings.ToLower(file.Name), ".jpeg") {

			rc, err := file.Open()
			if err != nil {
				i.log.Warn(fmt.Sprintf("failed to open file %s: %v", file.Name, err))
				continue
			}

			imageData, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				i.log.Warn(fmt.Sprintf("failed to read file %s: %v", file.Name, err))
				continue
			}

			// Convert to base64
			base64Data := fmt.Sprintf("data:image/jpeg;base64,%s",
				base64.StdEncoding.EncodeToString(imageData))
			images = append(images, base64Data)
		}
	}

	if len(images) == 0 {
		return nil, fmt.Errorf("no images found in zip")
	}

	return images, nil
}
