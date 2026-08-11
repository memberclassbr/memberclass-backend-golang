package lesson_pdf

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/memberclass-backend-golang/internal/shared/memberclasserrors"
)

type ProcessingJob struct {
	LessonID string
	Lesson   *Lesson
}

type ProcessingResult struct {
	LessonID string
	Result   *processResult
	Error    error
}

// requireLesson returns a 404 unless the lesson exists.
//
// This replaces a resolver that probed every configured database for the
// lesson and returned whichever one held it. With one database per deployment
// there is nothing to search, but callers still rely on being told "not found"
// before any work starts, so the existence check stays.
func (f *Feature) requireLesson(ctx context.Context, lessonID string) error {
	lessonData, err := f.GetByID(ctx, lessonID)
	if err != nil || lessonData == nil {
		return &memberclasserrors.MemberClassError{
			Code:    404,
			Message: "lesson not found",
		}
	}
	return nil
}

// ProcessLesson - Process a single lesson PDF
func (f *Feature) processLesson(ctx context.Context, lessonID string) (*processResult, error) {
	// 1. Reject unknown lessons before doing any work
	if err := f.requireLesson(ctx, lessonID); err != nil {
		return nil, err
	}

	f.log.Info(fmt.Sprintf("Processing lesson %s", lessonID))

	// 2. Get lesson with PDF asset
	lessonData, err := f.GetByIDWithPDFAsset(ctx, lessonID)
	if err != nil {
		if errors.Is(err, memberclasserrors.ErrLessonNotFound) {
			return nil, &memberclasserrors.MemberClassError{
				Code:    404,
				Message: "lesson not found",
			}
		}
		f.log.Error(fmt.Sprintf("Error getting lesson %s: %v", lessonID, err))
		return nil, &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "error getting lesson",
		}
	}

	// 3. Validate lesson has PDF
	if lessonData.MediaURL == nil || !strings.HasSuffix(*lessonData.MediaURL, ".pdf") {
		return nil, &memberclasserrors.MemberClassError{
			Code:    400,
			Message: "lesson does not have a PDF media URL",
		}
	}

	// 4. Create or update PDF asset
	asset, err := f.createOrUpdatePDFAsset(ctx, lessonID, *lessonData.MediaURL)
	if err != nil {
		f.log.Error(fmt.Sprintf("Error creating/updating PDF asset for lesson %s: %v", lessonID, err))
		return nil, &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "error creating PDF asset",
		}
	}

	// 5. Process PDF to images using the complete flow
	images, err := f.convertPdfToImages(*lessonData.MediaURL)
	if err != nil {
		errorMsg := err.Error()
		f.UpdatePDFAssetStatus(ctx, asset.ID, "failed", nil, &errorMsg)
		f.log.Error(fmt.Sprintf("Error converting PDF to images for lesson %s: %v", lessonID, err))
		return nil, &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "error converting PDF to images",
		}
	}

	// 6. Save pages directly
	processedPages, err := f.savePagesDirectlyWithRepo(ctx, asset.ID, lessonID, images)
	totalImages := len(images)
	if err != nil {
		errorMsg := err.Error()
		f.UpdatePDFAssetStatus(ctx, asset.ID, "partial", &totalImages, &errorMsg)
		f.log.Error(fmt.Sprintf("Error saving pages for lesson %s: %v", lessonID, err))
		return nil, &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "error saving pages",
		}
	}

	// 7. Update final status
	status := "done"
	if processedPages < len(images) {
		status = "partial"
	}

	err = f.UpdatePDFAssetStatus(ctx, asset.ID, status, &totalImages, nil)
	if err != nil {
		f.log.Error(fmt.Sprintf("Error updating PDF asset status for lesson %s: %v", lessonID, err))
		return nil, &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "error updating PDF asset status",
		}
	}

	return &processResult{
		Success:        processedPages > 0,
		TotalPages:     len(images),
		ProcessedPages: processedPages,
	}, nil
}

// ProcessAllPendingLessons - Process all pending PDF lessons across all databases
func (f *Feature) processAllPending(ctx context.Context, limit int) (*batchProcessResult, error) {
	allLessons, err := f.GetPendingPDFLessons(ctx, limit)
	if err != nil {
		f.log.Error(fmt.Sprintf("Error getting pending PDF lessons: %v", err))
		allLessons = nil
	} else {
		f.log.Info(fmt.Sprintf("Found %d pending lessons", len(allLessons)))
	}

	if len(allLessons) == 0 {
		return &batchProcessResult{
			Processed: 0,
			Total:     0,
			Results:   []processResult{},
		}, nil
	}

	// Process lessons concurrently using worker pool
	const maxWorkers = 5
	jobChan := make(chan ProcessingJob, len(allLessons))
	resultChan := make(chan ProcessingResult, len(allLessons))

	var wg sync.WaitGroup
	var mu sync.Mutex
	results := make([]processResult, 0, len(allLessons))
	processed := 0

	// Start workers
	for i := 0; i < maxWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobChan {
				select {
				case <-ctx.Done():
					return
				default:
					result, err := f.processLesson(ctx, job.LessonID)
					select {
					case resultChan <- ProcessingResult{LessonID: job.LessonID, Result: result, Error: err}:
					case <-ctx.Done():
						return
					}
				}
			}
		}()
	}

	// Send jobs
	go func() {
		defer close(jobChan)
		for _, lesson := range allLessons {
			select {
			case jobChan <- ProcessingJob{LessonID: *lesson.ID, Lesson: lesson}:
			case <-ctx.Done():
				return
			}
		}
	}()

	// Collect results
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// Process results
	for result := range resultChan {
		mu.Lock()
		if result.Error != nil {
			f.log.Error(fmt.Sprintf("Failed to process lesson %s: %v", result.LessonID, result.Error))
			results = append(results, processResult{
				Success: false,
				Error:   result.Error.Error(),
			})
		} else {
			results = append(results, *result.Result)
			if result.Result.Success {
				processed++
			}
		}
		mu.Unlock()
	}

	return &batchProcessResult{
		Processed: processed,
		Total:     len(allLessons),
		Results:   results,
	}, nil
}

// RetryFailedAssets - Retry processing failed PDF assets across all databases
func (f *Feature) retryFailedAssets(ctx context.Context, limit int) (*batchProcessResult, error) {
	allFailedAssets, err := f.GetFailedPDFAssets(ctx, limit)
	if err != nil {
		f.log.Error(fmt.Sprintf("Error getting failed PDF assets: %v", err))
		allFailedAssets = nil
	} else {
		f.log.Info(fmt.Sprintf("Found %d failed assets", len(allFailedAssets)))
	}

	if len(allFailedAssets) == 0 {
		return &batchProcessResult{
			Processed: 0,
			Total:     0,
			Results:   []processResult{},
		}, nil
	}

	// Retry assets concurrently using worker pool
	const maxWorkers = 3
	jobChan := make(chan LessonPDFAsset, len(allFailedAssets))
	resultChan := make(chan ProcessingResult, len(allFailedAssets))

	var wg sync.WaitGroup
	var mu sync.Mutex
	results := make([]processResult, 0, len(allFailedAssets))
	processed := 0

	// Start workers
	for i := 0; i < maxWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for asset := range jobChan {
				select {
				case <-ctx.Done():
					return
				default:
					result, err := f.processLesson(ctx, asset.LessonID)
					select {
					case resultChan <- ProcessingResult{LessonID: asset.LessonID, Result: result, Error: err}:
					case <-ctx.Done():
						return
					}
				}
			}
		}()
	}

	// Send jobs
	go func() {
		defer close(jobChan)
		for _, asset := range allFailedAssets {
			select {
			case jobChan <- *asset:
			case <-ctx.Done():
				return
			}
		}
	}()

	// Collect results
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// Process results
	for result := range resultChan {
		mu.Lock()
		if result.Error != nil {
			f.log.Error(fmt.Sprintf("Failed to retry asset %s: %v", result.LessonID, result.Error))
			results = append(results, processResult{
				Success: false,
				Error:   result.Error.Error(),
			})
		} else {
			results = append(results, *result.Result)
			if result.Result.Success {
				processed++
			}
		}
		mu.Unlock()
	}

	return &batchProcessResult{
		Processed: processed,
		Total:     len(allFailedAssets),
		Results:   results,
	}, nil
}

// isPermanentError checks if the error message indicates a permanent failure that will never succeed on retry
func isPermanentError(errorMsg *string) bool {
	if errorMsg == nil {
		return false
	}
	permanentPatterns := []string{
		"EmptyFile",
		"Damaged file",
		"Filesize exceeded",
		"TaskLimit",
		"403 Forbidden",
		"404 Not Found",
		"UploadError",
	}
	for _, pattern := range permanentPatterns {
		if strings.Contains(*errorMsg, pattern) {
			return true
		}
	}
	return false
}

// CleanupPermanentlyFailedAssets - Mark permanently failed assets
func (f *Feature) cleanupPermanentlyFailedAssets(ctx context.Context) (*cleanupFailedResponse, error) {
	var allResults []cleanupFailedResult
	totalFailed := 0
	removed := 0

	failedAssets, err := f.GetFailedPDFAssets(ctx, 0)
	if err != nil {
		f.log.Error(fmt.Sprintf("Error getting failed PDF assets: %v", err))
		failedAssets = nil
	}
	totalFailed += len(failedAssets)

	for _, asset := range failedAssets {
		if !isPermanentError(asset.Error) {
			continue
		}

		errorMsg := ""
		if asset.Error != nil {
			errorMsg = *asset.Error
		}

		// Delete orphaned pages first
		err := f.DeletePDFPagesByAssetID(ctx, asset.ID)
		if err != nil {
			f.log.Error(fmt.Sprintf("Error deleting pages for asset %s: %v", asset.ID, err))
		}

		// Mark as permanently_failed
		permanentErr := fmt.Sprintf("[permanently_failed] %s", errorMsg)
		err = f.UpdatePDFAssetStatus(ctx, asset.ID, "permanently_failed", nil, &permanentErr)
		if err != nil {
			f.log.Error(fmt.Sprintf("Error updating asset %s to permanently_failed: %v", asset.ID, err))
			continue
		}

		allResults = append(allResults, cleanupFailedResult{
			AssetID:  asset.ID,
			LessonID: asset.LessonID,
			Error:    errorMsg,
		})
		removed++
	}

	return &cleanupFailedResponse{
		Message: "Cleanup of permanently failed assets completed",
		Removed: removed,
		Total:   totalFailed,
		Results: allResults,
	}, nil
}

// CleanupOrphanedPages - Clean up orphaned PDF pages
func (f *Feature) cleanupOrphanedPages(ctx context.Context) error {

	failedAssets, err := f.GetFailedPDFAssets(ctx, 0)
	if err != nil {
		f.log.Error(fmt.Sprintf("Error getting failed PDF assets: %v", err))
		return nil
	}

	if len(failedAssets) == 0 {
		return nil
	}

	// Clean up pages concurrently using worker pool
	const maxWorkers = 3
	jobChan := make(chan LessonPDFAsset, len(failedAssets))
	resultChan := make(chan error, len(failedAssets))

	var wg sync.WaitGroup

	// Start workers
	for i := 0; i < maxWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for asset := range jobChan {
				select {
				case <-ctx.Done():
					return
				default:
					err := f.cleanupAssetPages(ctx, asset)
					select {
					case resultChan <- err:
					case <-ctx.Done():
						return
					}
				}
			}
		}()
	}

	// Send jobs
	go func() {
		defer close(jobChan)
		for _, asset := range failedAssets {
			select {
			case jobChan <- *asset:
			case <-ctx.Done():
				return
			}
		}
	}()

	// Collect results
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// Process results
	for err := range resultChan {
		if err != nil {
			f.log.Error(fmt.Sprintf("Failed to cleanup asset pages: %v", err))
		}
	}

	return nil
}

// cleanupAssetPages deletes every stored page image for one asset.
func (f *Feature) cleanupAssetPages(ctx context.Context, asset LessonPDFAsset) error {
	pages, err := f.GetPDFPagesByAssetID(ctx, asset.ID)
	if err != nil {
		f.log.Error(fmt.Sprintf("Error getting pages for asset %s: %v", asset.ID, err))
		return err
	}

	// Delete pages concurrently
	maxConcurrentDeletes := 5
	if maxConcurrentDeletes > len(pages) {
		maxConcurrentDeletes = len(pages)
	}

	if len(pages) == 0 {
		return nil
	}

	type deleteJob struct {
		pageID string
	}

	type deleteResult struct {
		pageID string
		err    error
	}

	jobChan := make(chan deleteJob, len(pages))
	resultChan := make(chan deleteResult, len(pages))

	var wg sync.WaitGroup

	// Start delete workers
	for i := 0; i < maxConcurrentDeletes; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobChan {
				select {
				case <-ctx.Done():
					return
				default:
					err := f.DeletePDFPage(ctx, job.pageID)
					select {
					case resultChan <- deleteResult{pageID: job.pageID, err: err}:
					case <-ctx.Done():
						return
					}
				}
			}
		}()
	}

	// Send delete jobs
	go func() {
		defer close(jobChan)
		for _, page := range pages {
			select {
			case jobChan <- deleteJob{pageID: page.ID}:
			case <-ctx.Done():
				return
			}
		}
	}()

	// Collect delete results
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// Process delete results
	for result := range resultChan {
		if result.err != nil {
			f.log.Error(fmt.Sprintf("Error deleting page %s: %v", result.pageID, result.err))
		}
	}

	return nil
}

// RegeneratePDF - Regenerate PDF processing for a lesson
func (f *Feature) regeneratePDF(ctx context.Context, lessonID string) error {
	// 1. Find the correct repository
	if err := f.requireLesson(ctx, lessonID); err != nil {
		return err
	}

	// 2. Get lesson with PDF asset
	lessonData, err := f.GetByIDWithPDFAsset(ctx, lessonID)
	if err != nil {
		if errors.Is(err, memberclasserrors.ErrLessonNotFound) {
			return &memberclasserrors.MemberClassError{
				Code:    404,
				Message: "lesson not found",
			}
		}
		f.log.Error(fmt.Sprintf("Error getting lesson %s: %v", lessonID, err))
		return &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "error getting lesson",
		}
	}

	// 3. Validate lesson has PDF
	if lessonData.MediaURL == nil || !strings.HasSuffix(*lessonData.MediaURL, ".pdf") {
		return &memberclasserrors.MemberClassError{
			Code:    400,
			Message: "lesson does not have a PDF media URL",
		}
	}

	// 4. Delete existing PDF asset and pages if exists
	if lessonData.PDFAsset != nil {
		// Delete all pages
		err = f.DeletePDFPagesByAssetID(ctx, lessonData.PDFAsset.ID)
		if err != nil {
			f.log.Error(fmt.Sprintf("Error deleting pages for asset %s: %v", lessonData.PDFAsset.ID, err))
		}

		// Reset asset status
		err = f.UpdatePDFAssetStatus(ctx, lessonData.PDFAsset.ID, "pending", nil, nil)
		if err != nil {
			f.log.Error(fmt.Sprintf("Error resetting asset %s: %v", lessonData.PDFAsset.ID, err))
		}
	} else {
		// Create new asset
		asset := &LessonPDFAsset{
			LessonID:     lessonID,
			SourcePDFURL: *lessonData.MediaURL,
			Status:       "pending",
		}
		err = f.CreatePDFAsset(ctx, asset)
		if err != nil {
			f.log.Error(fmt.Sprintf("Error creating PDF asset for lesson %s: %v", lessonID, err))
			return &memberclasserrors.MemberClassError{
				Code:    500,
				Message: "error creating PDF asset",
			}
		}
	}

	return nil
}

// ConvertPdfToImages - Complete PDF to images conversion flow
func (f *Feature) convertPdfToImages(pdfURL string) ([]string, error) {
	// 1. Get authentication token and create task (with automatic key rotation)
	token, task, err := f.pdf.GetTokenAndCreateTask()
	if err != nil {
		return nil, fmt.Errorf("failed to get token and create task: %w", err)
	}

	// 2. Add file
	serverFilename, err := f.pdf.AddFile(token, task.Task, pdfURL, task.Server)
	if err != nil {
		return nil, fmt.Errorf("failed to add file: %w", err)
	}

	// 3. Process task
	err = f.pdf.ProcessTask(token, task.Task, serverFilename, task.Server)
	if err != nil {
		return nil, fmt.Errorf("failed to process task: %w", err)
	}

	// 4. Download result
	zipData, err := f.pdf.DownloadTask(token, task.Task, task.Server)
	if err != nil {
		return nil, fmt.Errorf("failed to download task: %w", err)
	}

	// 5. Extract images
	images, err := f.pdf.ExtractImagesFromZip(zipData)
	if err != nil {
		return nil, fmt.Errorf("failed to extract images: %w", err)
	}

	return images, nil
}

// CreateOrUpdatePDFAsset - Create or update PDF asset (searches all databases)
func (f *Feature) createOrUpdatePDFAssetForLesson(ctx context.Context, lessonID, pdfURL string) (*LessonPDFAsset, error) {
	if err := f.requireLesson(ctx, lessonID); err != nil {
		return nil, err
	}
	return f.createOrUpdatePDFAsset(ctx, lessonID, pdfURL)
}

// createOrUpdatePDFAsset - Internal: create or update PDF asset using specific repo
func (f *Feature) createOrUpdatePDFAsset(ctx context.Context, lessonID, pdfURL string) (*LessonPDFAsset, error) {
	asset, err := f.GetPDFAssetByLessonID(ctx, lessonID)
	if err != nil && !errors.Is(err, memberclasserrors.ErrPDFAssetNotFound) {
		return nil, err
	}

	if asset == nil {
		// Create new asset with generated ID
		asset = &LessonPDFAsset{
			ID:           uuid.New().String(),
			LessonID:     lessonID,
			SourcePDFURL: pdfURL,
			Status:       "processing",
		}
		err = f.CreatePDFAsset(ctx, asset)
	} else {
		// Update existing asset
		asset.Status = "processing"
		asset.Error = nil
		err = f.UpdatePDFAsset(ctx, asset)
	}

	return asset, err
}

// saveSinglePage decodes one rendered page, uploads it and records the row.
// Safe for concurrent use: each call owns its page number.
func (f *Feature) saveSinglePage(ctx context.Context, assetID string, pageNumber int, imageBase64 string) (bool, error) {
	existingPage, err := f.GetPDFPageByAssetAndNumber(ctx, assetID, pageNumber)
	if err != nil && !errors.Is(err, memberclasserrors.ErrPDFPageNotFound) {
		return false, err
	}

	if existingPage != nil {
		return true, nil
	}

	// 1. Extract base64 data from data URL format if needed
	base64Data := imageBase64
	if strings.HasPrefix(imageBase64, "data:image/jpeg;base64,") {
		base64Data = strings.TrimPrefix(imageBase64, "data:image/jpeg;base64,")
	}

	// 2. Decode base64 to bytes
	imageData, err := base64.StdEncoding.DecodeString(base64Data)
	if err != nil {
		f.log.Error(fmt.Sprintf("Failed to decode base64 image for asset %s page %d: %v", assetID, pageNumber, err))
		return false, fmt.Errorf("failed to decode base64 image: %w", err)
	}

	// 3. Generate unique filename
	filename := fmt.Sprintf("lessons/%s/page-%d.jpg", assetID, pageNumber)

	// 4. Upload to this deployment's Spaces bucket
	imageURL, uploadErr := f.storage.Upload(ctx, imageData, filename, "image/jpeg")
	if uploadErr != nil {
		f.log.Error(fmt.Sprintf("Failed to upload page %d to storage for asset %s: %v", pageNumber, assetID, uploadErr))
		return false, fmt.Errorf("failed to upload image to storage: %w", uploadErr)
	}

	// 5. Save storage URL to database
	page := &LessonPDFPage{
		ID:         uuid.New().String(),
		AssetID:    assetID,
		PageNumber: pageNumber,
		ImageURL:   imageURL, // DigitalOcean Spaces URL
	}

	err = f.CreatePDFPage(ctx, page)
	if err != nil {
		f.log.Error(fmt.Sprintf("Failed to save page %d to database for asset %s: %v", pageNumber, assetID, err))
		return false, fmt.Errorf("failed to save page to database: %w", err)
	}

	return true, nil
}

// SavePagesDirectly - Save pages directly (public interface)
func (f *Feature) savePagesDirectly(ctx context.Context, assetID, lessonID string, images []string) (int, error) {
	if err := f.requireLesson(ctx, lessonID); err != nil {
		return 0, err
	}
	return f.savePagesDirectlyWithRepo(ctx, assetID, lessonID, images)
}

// savePagesDirectlyWithRepo - Internal: save pages using specific repo
func (f *Feature) savePagesDirectlyWithRepo(ctx context.Context, assetID, lessonID string, images []string) (int, error) {
	if len(images) == 0 {
		return 0, nil
	}

	// Save pages concurrently with controlled concurrency
	const maxConcurrent = 5
	maxConcurrentWorkers := maxConcurrent
	if maxConcurrentWorkers > len(images) {
		maxConcurrentWorkers = len(images)
	}

	type pageJob struct {
		index       int
		imageBase64 string
		pageNumber  int
	}

	type pageResult struct {
		index      int
		pageNumber int
		success    bool
		err        error
	}

	jobChan := make(chan pageJob, len(images))
	resultChan := make(chan pageResult, len(images))

	var wg sync.WaitGroup
	var mu sync.Mutex
	processedPages := 0

	// Start workers
	for i := 0; i < maxConcurrentWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobChan {
				select {
				case <-ctx.Done():
					return
				default:
					success, err := f.saveSinglePage(ctx, assetID, job.pageNumber, job.imageBase64)
					select {
					case resultChan <- pageResult{
						index:      job.index,
						pageNumber: job.pageNumber,
						success:    success,
						err:        err,
					}:
					case <-ctx.Done():
						return
					}
				}
			}
		}()
	}

	// Send jobs
	go func() {
		defer close(jobChan)
		for i, imageBase64 := range images {
			select {
			case jobChan <- pageJob{
				index:       i,
				imageBase64: imageBase64,
				pageNumber:  i + 1,
			}:
			case <-ctx.Done():
				return
			}
		}
	}()

	// Collect results
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// Process results
	for result := range resultChan {
		mu.Lock()
		if result.success {
			processedPages++
		} else if result.err != nil {
			f.log.Error(fmt.Sprintf("Failed to save page %d: %v", result.pageNumber, result.err))
		}
		mu.Unlock()
	}

	f.log.Info(fmt.Sprintf("Pages saved for asset %s: %d/%d", assetID, processedPages, len(images)))

	return processedPages, nil
}

// ValidateLessonHasPDF - Validate that lesson has a PDF media URL (searches all databases)
func (f *Feature) validateLessonHasPDF(ctx context.Context, lessonID string) error {
	if err := f.requireLesson(ctx, lessonID); err != nil {
		return err
	}

	lesson, err := f.GetByID(ctx, lessonID)
	if err != nil {
		if errors.Is(err, memberclasserrors.ErrLessonNotFound) {
			return &memberclasserrors.MemberClassError{
				Code:    404,
				Message: "lesson not found",
			}
		}
		f.log.Error(fmt.Sprintf("Error getting lesson %s: %v", lessonID, err))
		return &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "error getting lesson",
		}
	}

	if lesson.MediaURL == nil || !strings.HasSuffix(*lesson.MediaURL, ".pdf") {
		return &memberclasserrors.MemberClassError{
			Code:    400,
			Message: "lesson does not have a PDF media URL",
		}
	}

	return nil
}

// GetLessonWithPDFAsset - Get lesson with PDF asset relationship (searches all databases)
func (f *Feature) getLessonWithPDFAsset(ctx context.Context, lessonID string) (*Lesson, error) {
	if err := f.requireLesson(ctx, lessonID); err != nil {
		return nil, err
	}

	lesson, err := f.GetByIDWithPDFAsset(ctx, lessonID)
	if err != nil {
		if errors.Is(err, memberclasserrors.ErrLessonNotFound) {
			return nil, &memberclasserrors.MemberClassError{
				Code:    404,
				Message: "lesson not found",
			}
		}
		f.log.Error(fmt.Sprintf("Error getting lesson %s: %v", lessonID, err))
		return nil, &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "error getting lesson",
		}
	}

	return lesson, nil
}

// GetPDFPagesByAssetID - Get PDF pages by asset ID (searches all databases)
