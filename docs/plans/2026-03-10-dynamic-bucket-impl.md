# Dynamic Bucket Resolution — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Make PDF processing upload images to the same DO Spaces bucket the source PDF came from, instead of a hardcoded bucket.

**Architecture:** Extract bucket name from the `lesson.MediaURL` hostname at processing time. Propagate it through `ProcessLesson` → `SavePagesDirectly` → `saveSinglePage` → `UploadToBucket`. Keep `Upload` unchanged for backward compatibility.

**Tech Stack:** Go, AWS SDK v2 (S3), DigitalOcean Spaces

---

### Task 1: Add `extractBucketFromURL` helper and tests

**Files:**
- Modify: `internal/infrastructure/adapters/storage/digital_ocean_spaces.go`
- Modify: `internal/infrastructure/adapters/storage/digital_ocean_spaces_test.go`

**Step 1: Write the failing test**

Add to `digital_ocean_spaces_test.go`:

```go
func TestExtractBucketFromURL(t *testing.T) {
	os.Setenv("DO_SPACES_ID", "invalid")
	os.Setenv("DO_SPACES_SECRET", "invalid")
	os.Setenv("DO_SPACES_BUCKET", "default-bucket")
	os.Setenv("DO_SPACES_URL", "https://nyc3.digitaloceanspaces.com")

	mockLogger := &MockLogger{}
	service, _ := NewDigitalOceanSpaces(mockLogger)
	dos := service.(*DigitalOceanSpaces)

	tests := []struct {
		name     string
		url      string
		expected string
	}{
		{"Full DO Spaces URL", "https://my-bucket.nyc3.digitaloceanspaces.com/path/file.pdf", "my-bucket"},
		{"Different bucket", "https://other-bucket.nyc3.digitaloceanspaces.com/lessons/abc/page-1.jpg", "other-bucket"},
		{"Just a key (no URL)", "lessons/abc/page-1.jpg", "default-bucket"},
		{"Empty string", "", "default-bucket"},
		{"Non-DO URL", "https://example.com/file.pdf", "example"},
		{"URL without path", "https://my-bucket.nyc3.digitaloceanspaces.com", "my-bucket"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := dos.extractBucketFromURL(tt.url)
			assert.Equal(t, tt.expected, result)
		})
	}
}
```

**Step 2: Run test to verify it fails**

Run: `cd /home/micael/dev/grupoon/memberclass-backend-golang && go test ./internal/infrastructure/adapters/storage/ -run TestExtractBucketFromURL -v`
Expected: FAIL — `dos.extractBucketFromURL undefined`

**Step 3: Write the implementation**

Add to `digital_ocean_spaces.go`, after the existing `extractKeyFromURL` method:

```go
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
```

**Step 4: Run test to verify it passes**

Run: `cd /home/micael/dev/grupoon/memberclass-backend-golang && go test ./internal/infrastructure/adapters/storage/ -run TestExtractBucketFromURL -v`
Expected: PASS

---

### Task 2: Add `UploadToBucket` to the `Storage` interface

**Files:**
- Modify: `internal/domain/ports/storage.go`

**Step 1: Add the new method to the interface**

Replace the full interface in `storage.go`:

```go
type Storage interface {
	Upload(ctx context.Context, data []byte, filename string, contentType string) (string, error)
	UploadToBucket(ctx context.Context, bucket string, data []byte, filename string, contentType string) (string, error)
	Download(ctx context.Context, urlOrKey string) ([]byte, error)
	Delete(ctx context.Context, urlOrKey string) error
	Exists(ctx context.Context, urlOrKey string) (bool, error)
}
```

**Step 2: Verify build fails (missing implementation)**

Run: `cd /home/micael/dev/grupoon/memberclass-backend-golang && go build ./...`
Expected: FAIL — `DigitalOceanSpaces does not implement Storage (missing UploadToBucket method)`

---

### Task 3: Implement `UploadToBucket` in `DigitalOceanSpaces`

**Files:**
- Modify: `internal/infrastructure/adapters/storage/digital_ocean_spaces.go`
- Modify: `internal/infrastructure/adapters/storage/digital_ocean_spaces_test.go`

**Step 1: Write the failing test**

Add to `digital_ocean_spaces_test.go`:

```go
func TestDigitalOceanSpaces_UploadToBucket_InvalidCredentials(t *testing.T) {
	os.Setenv("DO_SPACES_ID", "invalid")
	os.Setenv("DO_SPACES_SECRET", "invalid")
	os.Setenv("DO_SPACES_BUCKET", "default-bucket")
	os.Setenv("DO_SPACES_URL", "https://sfo3.digitaloceanspaces.com")

	mockLogger := &MockLogger{}
	service, _ := NewDigitalOceanSpaces(mockLogger)

	ctx := context.Background()
	data := []byte("test data")
	filename := "test.jpg"
	contentType := "image/jpeg"

	_, err := service.UploadToBucket(ctx, "custom-bucket", data, filename, contentType)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to upload file")
}
```

**Step 2: Run test to verify it fails**

Run: `cd /home/micael/dev/grupoon/memberclass-backend-golang && go test ./internal/infrastructure/adapters/storage/ -run TestDigitalOceanSpaces_UploadToBucket -v`
Expected: FAIL — `service.UploadToBucket undefined`

**Step 3: Implement `UploadToBucket`**

Add to `digital_ocean_spaces.go`, right after the existing `Upload` method (after line 91):

```go
func (d *DigitalOceanSpaces) UploadToBucket(ctx context.Context, bucket string, data []byte, filename string, contentType string) (string, error) {
	d.logger.Info("Uploading file to DigitalOcean Spaces", "filename", filename, "bucket", bucket, "size", len(data))

	input := &s3.PutObjectInput{
		Bucket:      aws.String(bucket),
		Key:         aws.String(filename),
		Body:        bytes.NewReader(data),
		ContentType: aws.String(contentType),
		ACL:         types.ObjectCannedACLPublicRead,
	}
	_, err := d.client.PutObject(ctx, input)
	if err != nil {
		d.logger.Error("Failed to upload file to DigitalOcean Spaces", "filename", filename, "bucket", bucket, "error", err)
		return "", fmt.Errorf("failed to upload file: %w", err)
	}

	publicURL := fmt.Sprintf("https://%s.%s.digitaloceanspaces.com/%s", bucket, d.region, filename)
	d.logger.Info("File uploaded successfully", "filename", filename, "bucket", bucket, "url", publicURL)

	return publicURL, nil
}
```

**Step 4: Run test to verify it passes**

Run: `cd /home/micael/dev/grupoon/memberclass-backend-golang && go test ./internal/infrastructure/adapters/storage/ -run TestDigitalOceanSpaces_UploadToBucket -v`
Expected: PASS

**Step 5: Verify full build compiles**

Run: `cd /home/micael/dev/grupoon/memberclass-backend-golang && go build ./...`
Expected: PASS

---

### Task 4: Update `Download`, `Delete`, `Exists` to resolve bucket from URL

**Files:**
- Modify: `internal/infrastructure/adapters/storage/digital_ocean_spaces.go`

**Step 1: Update `Download` method**

Replace line 99 (`Bucket: aws.String(d.bucket),`) with:

```go
	bucket := d.extractBucketFromURL(urlOrKey)
```

And use `bucket` in the input:

```go
	input := &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	}
```

Also update the log line to include bucket:

```go
	d.logger.Info("Downloading file from DigitalOcean Spaces", "key", key, "bucket", bucket)
```

**Step 2: Update `Delete` method**

Same pattern — replace `d.bucket` with `d.extractBucketFromURL(urlOrKey)`:

```go
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
```

**Step 3: Update `Exists` method**

Same pattern:

```go
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
		if err != nil && !strings.Contains(err.Error(), "NotFound") {
			d.logger.Error("Error checking file existence", "key", key, "bucket", bucket, "error", err)
			return false, fmt.Errorf("failed to check file existence: %w", err)
		}
		return false, nil
	}

	d.logger.Debug("File exists", "key", key, "bucket", bucket)
	return true, nil
}
```

**Step 4: Run all storage tests**

Run: `cd /home/micael/dev/grupoon/memberclass-backend-golang && go test ./internal/infrastructure/adapters/storage/ -v`
Expected: All PASS

---

### Task 5: Propagate bucket through the PDF processing pipeline

**Files:**
- Modify: `internal/domain/usecases/lessons/pdf_processor_usecase.go`

**Step 1: Add `extractBucketFromMediaURL` helper to the use case**

Add at the bottom of `pdf_processor_usecase.go`:

```go
// extractBucketFromMediaURL extracts the bucket name from a DO Spaces URL.
// Returns empty string if the URL is not a valid DO Spaces URL, letting the
// storage layer fall back to the default bucket.
func extractBucketFromMediaURL(mediaURL string) string {
	if !strings.HasPrefix(mediaURL, "http") {
		return ""
	}

	parsed, err := url.Parse(mediaURL)
	if err != nil {
		return ""
	}

	host := parsed.Hostname()
	parts := strings.SplitN(host, ".", 2)
	if len(parts) < 2 {
		return ""
	}

	return parts[0]
}
```

Add `"net/url"` to the imports block.

**Step 2: Update `saveSinglePage` — add `bucket` parameter**

Change the signature from:

```go
func (u *pdfProcessorUseCase) saveSinglePage(ctx context.Context, assetID string, pageNumber int, imageBase64 string) (bool, error) {
```

To:

```go
func (u *pdfProcessorUseCase) saveSinglePage(ctx context.Context, assetID string, pageNumber int, imageBase64 string, bucket string) (bool, error) {
```

Change the upload call (line 675) from:

```go
imageURL, err := u.storageService.Upload(ctx, imageData, filename, "image/jpeg")
```

To:

```go
var imageURL string
var uploadErr error
if bucket != "" {
	imageURL, uploadErr = u.storageService.UploadToBucket(ctx, bucket, imageData, filename, "image/jpeg")
} else {
	imageURL, uploadErr = u.storageService.Upload(ctx, imageData, filename, "image/jpeg")
}
if uploadErr != nil {
	u.logger.Error(fmt.Sprintf("Failed to upload page %d to storage for asset %s: %v", pageNumber, assetID, uploadErr))
	return false, fmt.Errorf("failed to upload image to storage: %w", uploadErr)
}
```

Remove the old error handling block that follows (lines 676-679).

**Step 3: Update `SavePagesDirectly` — add `bucket` parameter**

Change the signature from:

```go
func (u *pdfProcessorUseCase) SavePagesDirectly(ctx context.Context, assetID, lessonID string, images []string) (int, error) {
```

To:

```go
func (u *pdfProcessorUseCase) SavePagesDirectly(ctx context.Context, assetID, lessonID string, images []string, bucket string) (int, error) {
```

Update the `saveSinglePage` call inside the worker goroutine (line 743) from:

```go
success, err := u.saveSinglePage(ctx, assetID, job.pageNumber, job.imageBase64)
```

To:

```go
success, err := u.saveSinglePage(ctx, assetID, job.pageNumber, job.imageBase64, bucket)
```

**Step 4: Update `ProcessLesson` — extract bucket and propagate**

After line 68 (`asset, err := u.CreateOrUpdatePDFAsset(...)`) and before the `ConvertPdfToImages` call, add:

```go
	// Extract bucket from lesson media URL for dynamic storage routing
	bucket := extractBucketFromMediaURL(*lesson.MediaURL)
	u.logger.Info(fmt.Sprintf("Resolved bucket '%s' from media URL for lesson %s", bucket, lessonID))
```

Update the `SavePagesDirectly` call (line 90) from:

```go
processedPages, err := u.SavePagesDirectly(ctx, asset.ID, lessonID, images)
```

To:

```go
processedPages, err := u.SavePagesDirectly(ctx, asset.ID, lessonID, images, bucket)
```

**Step 5: Verify full build compiles**

Run: `cd /home/micael/dev/grupoon/memberclass-backend-golang && go build ./...`
Expected: PASS

**Step 6: Run all tests**

Run: `cd /home/micael/dev/grupoon/memberclass-backend-golang && go test ./...`
Expected: All PASS

---

### Task 6: Commit all changes

**Step 1: Review changes**

Run: `cd /home/micael/dev/grupoon/memberclass-backend-golang && git diff --stat`

Expected modified files:
- `docs/plans/2026-03-10-dynamic-bucket-design.md` (new)
- `docs/plans/2026-03-10-dynamic-bucket-impl.md` (new)
- `internal/domain/ports/storage.go`
- `internal/infrastructure/adapters/storage/digital_ocean_spaces.go`
- `internal/infrastructure/adapters/storage/digital_ocean_spaces_test.go`
- `internal/domain/usecases/lessons/pdf_processor_usecase.go`

**Step 2: Commit**

```bash
git add \
  docs/plans/2026-03-10-dynamic-bucket-design.md \
  docs/plans/2026-03-10-dynamic-bucket-impl.md \
  internal/domain/ports/storage.go \
  internal/infrastructure/adapters/storage/digital_ocean_spaces.go \
  internal/infrastructure/adapters/storage/digital_ocean_spaces_test.go \
  internal/domain/usecases/lessons/pdf_processor_usecase.go

git commit -m "feat(storage): add dynamic bucket resolution from PDF source URL

Extracts bucket name from lesson.MediaURL hostname so processed
images are uploaded to the same bucket the source PDF came from.
DO_SPACES_BUCKET remains as the default fallback."
```
