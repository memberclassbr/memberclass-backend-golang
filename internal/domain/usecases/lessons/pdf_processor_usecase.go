package lessons

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/memberclass-backend-golang/internal/domain/dto"
	"github.com/memberclass-backend-golang/internal/domain/entities/lessons"
	"github.com/memberclass-backend-golang/internal/domain/ports"
	"github.com/memberclass-backend-golang/internal/domain/ports/lesson"
	"github.com/memberclass-backend-golang/internal/domain/ports/pdf_processor"
	"github.com/memberclass-backend-golang/internal/shared/memberclasserrors"
)

type pdfProcessorUseCase struct {
	repo           lesson.LessonRepository
	pdfService     pdf_processor.PdfProcessService
	storageService ports.Storage
	logger         ports.Logger
}

type ProcessingJob struct {
	LessonID string
	Lesson   *lessons.Lesson
}

type ProcessingResult struct {
	LessonID string
	Result   *dto.ProcessResult
	Error    error
}

func NewPdfProcessorUseCase(
	repo lesson.LessonRepository,
	pdfService pdf_processor.PdfProcessService,
	storageService ports.Storage,
	logger ports.Logger,
) pdf_processor.PdfProcessorUseCase {
	return &pdfProcessorUseCase{
		repo:           repo,
		pdfService:     pdfService,
		storageService: storageService,
		logger:         logger,
	}
}

// requireLesson returns a 404 unless the lesson exists.
//
// This replaces a resolver that probed every configured database for the
// lesson and returned whichever one held it. With one database per deployment
// there is nothing to search, but callers still rely on being told "not found"
// before any work starts, so the existence check stays.
func (u *pdfProcessorUseCase) requireLesson(ctx context.Context, lessonID string) error {
	lessonData, err := u.repo.GetByID(ctx, lessonID)
	if err != nil || lessonData == nil {
		return &memberclasserrors.MemberClassError{
			Code:    404,
			Message: "lesson not found",
		}
	}
	return nil
}

// ProcessLesson - Process a single lesson PDF
func (u *pdfProcessorUseCase) ProcessLesson(ctx context.Context, lessonID string) (*dto.ProcessResult, error) {
	// 1. Reject unknown lessons before doing any work
	if err := u.requireLesson(ctx, lessonID); err != nil {
		return nil, err
	}
	repo := u.repo

	u.logger.Info(fmt.Sprintf("Processing lesson %s", lessonID))

	// 2. Get lesson with PDF asset
	lessonData, err := repo.GetByIDWithPDFAsset(ctx, lessonID)
	if err != nil {
		if errors.Is(err, memberclasserrors.ErrLessonNotFound) {
			return nil, &memberclasserrors.MemberClassError{
				Code:    404,
				Message: "lesson not found",
			}
		}
		u.logger.Error(fmt.Sprintf("Error getting lesson %s: %v", lessonID, err))
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
	asset, err := u.createOrUpdatePDFAsset(ctx, repo, lessonID, *lessonData.MediaURL)
	if err != nil {
		u.logger.Error(fmt.Sprintf("Error creating/updating PDF asset for lesson %s: %v", lessonID, err))
		return nil, &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "error creating PDF asset",
		}
	}

	// 5. Process PDF to images using the complete flow
	images, err := u.ConvertPdfToImages(*lessonData.MediaURL)
	if err != nil {
		errorMsg := err.Error()
		repo.UpdatePDFAssetStatus(ctx, asset.ID, "failed", nil, &errorMsg)
		u.logger.Error(fmt.Sprintf("Error converting PDF to images for lesson %s: %v", lessonID, err))
		return nil, &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "error converting PDF to images",
		}
	}

	// 6. Save pages directly
	processedPages, err := u.savePagesDirectlyWithRepo(ctx, repo, asset.ID, lessonID, images)
	totalImages := len(images)
	if err != nil {
		errorMsg := err.Error()
		repo.UpdatePDFAssetStatus(ctx, asset.ID, "partial", &totalImages, &errorMsg)
		u.logger.Error(fmt.Sprintf("Error saving pages for lesson %s: %v", lessonID, err))
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

	err = repo.UpdatePDFAssetStatus(ctx, asset.ID, status, &totalImages, nil)
	if err != nil {
		u.logger.Error(fmt.Sprintf("Error updating PDF asset status for lesson %s: %v", lessonID, err))
		return nil, &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "error updating PDF asset status",
		}
	}

	return &dto.ProcessResult{
		Success:        processedPages > 0,
		TotalPages:     len(images),
		ProcessedPages: processedPages,
	}, nil
}

// ProcessAllPendingLessons - Process all pending PDF lessons across all databases
func (u *pdfProcessorUseCase) ProcessAllPendingLessons(ctx context.Context, limit int) (*dto.BatchProcessResult, error) {
	allLessons, err := u.repo.GetPendingPDFLessons(ctx, limit)
	if err != nil {
		u.logger.Error(fmt.Sprintf("Error getting pending PDF lessons: %v", err))
		allLessons = nil
	} else {
		u.logger.Info(fmt.Sprintf("Found %d pending lessons", len(allLessons)))
	}

	if len(allLessons) == 0 {
		return &dto.BatchProcessResult{
			Processed: 0,
			Total:     0,
			Results:   []dto.ProcessResult{},
		}, nil
	}

	// Process lessons concurrently using worker pool
	const maxWorkers = 5
	jobChan := make(chan ProcessingJob, len(allLessons))
	resultChan := make(chan ProcessingResult, len(allLessons))

	var wg sync.WaitGroup
	var mu sync.Mutex
	results := make([]dto.ProcessResult, 0, len(allLessons))
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
					result, err := u.ProcessLesson(ctx, job.LessonID)
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
			u.logger.Error(fmt.Sprintf("Failed to process lesson %s: %v", result.LessonID, result.Error))
			results = append(results, dto.ProcessResult{
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

	return &dto.BatchProcessResult{
		Processed: processed,
		Total:     len(allLessons),
		Results:   results,
	}, nil
}

// RetryFailedAssets - Retry processing failed PDF assets across all databases
func (u *pdfProcessorUseCase) RetryFailedAssets(ctx context.Context, limit int) (*dto.BatchProcessResult, error) {
	allFailedAssets, err := u.repo.GetFailedPDFAssets(ctx, limit)
	if err != nil {
		u.logger.Error(fmt.Sprintf("Error getting failed PDF assets: %v", err))
		allFailedAssets = nil
	} else {
		u.logger.Info(fmt.Sprintf("Found %d failed assets", len(allFailedAssets)))
	}

	if len(allFailedAssets) == 0 {
		return &dto.BatchProcessResult{
			Processed: 0,
			Total:     0,
			Results:   []dto.ProcessResult{},
		}, nil
	}

	// Retry assets concurrently using worker pool
	const maxWorkers = 3
	jobChan := make(chan lessons.LessonPDFAsset, len(allFailedAssets))
	resultChan := make(chan ProcessingResult, len(allFailedAssets))

	var wg sync.WaitGroup
	var mu sync.Mutex
	results := make([]dto.ProcessResult, 0, len(allFailedAssets))
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
					result, err := u.ProcessLesson(ctx, asset.LessonID)
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
			u.logger.Error(fmt.Sprintf("Failed to retry asset %s: %v", result.LessonID, result.Error))
			results = append(results, dto.ProcessResult{
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

	return &dto.BatchProcessResult{
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
func (u *pdfProcessorUseCase) CleanupPermanentlyFailedAssets(ctx context.Context) (*dto.CleanupFailedResponse, error) {
	var allResults []dto.CleanupFailedResult
	totalFailed := 0
	removed := 0

	repo := u.repo

	failedAssets, err := repo.GetFailedPDFAssets(ctx, 0)
	if err != nil {
		u.logger.Error(fmt.Sprintf("Error getting failed PDF assets: %v", err))
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
		err := repo.DeletePDFPagesByAssetID(ctx, asset.ID)
		if err != nil {
			u.logger.Error(fmt.Sprintf("Error deleting pages for asset %s: %v", asset.ID, err))
		}

		// Mark as permanently_failed
		permanentErr := fmt.Sprintf("[permanently_failed] %s", errorMsg)
		err = repo.UpdatePDFAssetStatus(ctx, asset.ID, "permanently_failed", nil, &permanentErr)
		if err != nil {
			u.logger.Error(fmt.Sprintf("Error updating asset %s to permanently_failed: %v", asset.ID, err))
			continue
		}

		allResults = append(allResults, dto.CleanupFailedResult{
			AssetID:  asset.ID,
			LessonID: asset.LessonID,
			Error:    errorMsg,
		})
		removed++
	}

	return &dto.CleanupFailedResponse{
		Message: "Cleanup of permanently failed assets completed",
		Removed: removed,
		Total:   totalFailed,
		Results: allResults,
	}, nil
}

// CleanupOrphanedPages - Clean up orphaned PDF pages
func (u *pdfProcessorUseCase) CleanupOrphanedPages(ctx context.Context) error {
	repo := u.repo

	failedAssets, err := repo.GetFailedPDFAssets(ctx, 0)
	if err != nil {
		u.logger.Error(fmt.Sprintf("Error getting failed PDF assets: %v", err))
		return nil
	}

	if len(failedAssets) == 0 {
		return nil
	}

	// Clean up pages concurrently using worker pool
	const maxWorkers = 3
	jobChan := make(chan lessons.LessonPDFAsset, len(failedAssets))
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
					err := u.cleanupAssetPages(ctx, repo, asset)
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
			u.logger.Error(fmt.Sprintf("Failed to cleanup asset pages: %v", err))
		}
	}

	return nil
}

// cleanupAssetPages - Clean up pages for a single asset using the provided repo
func (u *pdfProcessorUseCase) cleanupAssetPages(ctx context.Context, repo lesson.LessonRepository, asset lessons.LessonPDFAsset) error {
	pages, err := repo.GetPDFPagesByAssetID(ctx, asset.ID)
	if err != nil {
		u.logger.Error(fmt.Sprintf("Error getting pages for asset %s: %v", asset.ID, err))
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
					err := repo.DeletePDFPage(ctx, job.pageID)
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
			u.logger.Error(fmt.Sprintf("Error deleting page %s: %v", result.pageID, result.err))
		}
	}

	return nil
}

// RegeneratePDF - Regenerate PDF processing for a lesson
func (u *pdfProcessorUseCase) RegeneratePDF(ctx context.Context, lessonID string) error {
	// 1. Find the correct repository
	if err := u.requireLesson(ctx, lessonID); err != nil {
		return err
	}
	repo := u.repo

	// 2. Get lesson with PDF asset
	lessonData, err := repo.GetByIDWithPDFAsset(ctx, lessonID)
	if err != nil {
		if errors.Is(err, memberclasserrors.ErrLessonNotFound) {
			return &memberclasserrors.MemberClassError{
				Code:    404,
				Message: "lesson not found",
			}
		}
		u.logger.Error(fmt.Sprintf("Error getting lesson %s: %v", lessonID, err))
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
		err = repo.DeletePDFPagesByAssetID(ctx, lessonData.PDFAsset.ID)
		if err != nil {
			u.logger.Error(fmt.Sprintf("Error deleting pages for asset %s: %v", lessonData.PDFAsset.ID, err))
		}

		// Reset asset status
		err = repo.UpdatePDFAssetStatus(ctx, lessonData.PDFAsset.ID, "pending", nil, nil)
		if err != nil {
			u.logger.Error(fmt.Sprintf("Error resetting asset %s: %v", lessonData.PDFAsset.ID, err))
		}
	} else {
		// Create new asset
		asset := &lessons.LessonPDFAsset{
			LessonID:     lessonID,
			SourcePDFURL: *lessonData.MediaURL,
			Status:       "pending",
		}
		err = repo.CreatePDFAsset(ctx, asset)
		if err != nil {
			u.logger.Error(fmt.Sprintf("Error creating PDF asset for lesson %s: %v", lessonID, err))
			return &memberclasserrors.MemberClassError{
				Code:    500,
				Message: "error creating PDF asset",
			}
		}
	}

	return nil
}

// ConvertPdfToImages - Complete PDF to images conversion flow
func (u *pdfProcessorUseCase) ConvertPdfToImages(pdfURL string) ([]string, error) {
	// 1. Get authentication token and create task (with automatic key rotation)
	token, task, err := u.pdfService.GetTokenAndCreateTask()
	if err != nil {
		return nil, fmt.Errorf("failed to get token and create task: %w", err)
	}

	// 2. Add file
	serverFilename, err := u.pdfService.AddFile(token, task.Task, pdfURL, task.Server)
	if err != nil {
		return nil, fmt.Errorf("failed to add file: %w", err)
	}

	// 3. Process task
	err = u.pdfService.ProcessTask(token, task.Task, serverFilename, task.Server)
	if err != nil {
		return nil, fmt.Errorf("failed to process task: %w", err)
	}

	// 4. Download result
	zipData, err := u.pdfService.DownloadTask(token, task.Task, task.Server)
	if err != nil {
		return nil, fmt.Errorf("failed to download task: %w", err)
	}

	// 5. Extract images
	images, err := u.pdfService.ExtractImagesFromZip(zipData)
	if err != nil {
		return nil, fmt.Errorf("failed to extract images: %w", err)
	}

	return images, nil
}

// CreateOrUpdatePDFAsset - Create or update PDF asset (searches all databases)
func (u *pdfProcessorUseCase) CreateOrUpdatePDFAsset(ctx context.Context, lessonID, pdfURL string) (*lessons.LessonPDFAsset, error) {
	if err := u.requireLesson(ctx, lessonID); err != nil {
		return nil, err
	}
	repo := u.repo
	return u.createOrUpdatePDFAsset(ctx, repo, lessonID, pdfURL)
}

// createOrUpdatePDFAsset - Internal: create or update PDF asset using specific repo
func (u *pdfProcessorUseCase) createOrUpdatePDFAsset(ctx context.Context, repo lesson.LessonRepository, lessonID, pdfURL string) (*lessons.LessonPDFAsset, error) {
	asset, err := repo.GetPDFAssetByLessonID(ctx, lessonID)
	if err != nil && !errors.Is(err, memberclasserrors.ErrPDFAssetNotFound) {
		return nil, err
	}

	if asset == nil {
		// Create new asset with generated ID
		asset = &lessons.LessonPDFAsset{
			ID:           uuid.New().String(),
			LessonID:     lessonID,
			SourcePDFURL: pdfURL,
			Status:       "processing",
		}
		err = repo.CreatePDFAsset(ctx, asset)
	} else {
		// Update existing asset
		asset.Status = "processing"
		asset.Error = nil
		err = repo.UpdatePDFAsset(ctx, asset)
	}

	return asset, err
}

// saveSinglePage - Save a single page using specific repo (thread-safe)
func (u *pdfProcessorUseCase) saveSinglePage(ctx context.Context, repo lesson.LessonRepository, assetID string, pageNumber int, imageBase64 string) (bool, error) {
	existingPage, err := repo.GetPDFPageByAssetAndNumber(ctx, assetID, pageNumber)
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
		u.logger.Error(fmt.Sprintf("Failed to decode base64 image for asset %s page %d: %v", assetID, pageNumber, err))
		return false, fmt.Errorf("failed to decode base64 image: %w", err)
	}

	// 3. Generate unique filename
	filename := fmt.Sprintf("lessons/%s/page-%d.jpg", assetID, pageNumber)

	// 4. Upload to this deployment's Spaces bucket
	imageURL, uploadErr := u.storageService.Upload(ctx, imageData, filename, "image/jpeg")
	if uploadErr != nil {
		u.logger.Error(fmt.Sprintf("Failed to upload page %d to storage for asset %s: %v", pageNumber, assetID, uploadErr))
		return false, fmt.Errorf("failed to upload image to storage: %w", uploadErr)
	}

	// 5. Save storage URL to database
	page := &lessons.LessonPDFPage{
		ID:         uuid.New().String(),
		AssetID:    assetID,
		PageNumber: pageNumber,
		ImageURL:   imageURL, // DigitalOcean Spaces URL
	}

	err = repo.CreatePDFPage(ctx, page)
	if err != nil {
		u.logger.Error(fmt.Sprintf("Failed to save page %d to database for asset %s: %v", pageNumber, assetID, err))
		return false, fmt.Errorf("failed to save page to database: %w", err)
	}

	return true, nil
}

// SavePagesDirectly - Save pages directly (public interface)
func (u *pdfProcessorUseCase) SavePagesDirectly(ctx context.Context, assetID, lessonID string, images []string) (int, error) {
	if err := u.requireLesson(ctx, lessonID); err != nil {
		return 0, err
	}
	repo := u.repo
	return u.savePagesDirectlyWithRepo(ctx, repo, assetID, lessonID, images)
}

// savePagesDirectlyWithRepo - Internal: save pages using specific repo
func (u *pdfProcessorUseCase) savePagesDirectlyWithRepo(ctx context.Context, repo lesson.LessonRepository, assetID, lessonID string, images []string) (int, error) {
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
					success, err := u.saveSinglePage(ctx, repo, assetID, job.pageNumber, job.imageBase64)
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
			u.logger.Error(fmt.Sprintf("Failed to save page %d: %v", result.pageNumber, result.err))
		}
		mu.Unlock()
	}

	u.logger.Info(fmt.Sprintf("Pages saved for asset %s: %d/%d", assetID, processedPages, len(images)))

	return processedPages, nil
}

// ValidateLessonHasPDF - Validate that lesson has a PDF media URL (searches all databases)
func (u *pdfProcessorUseCase) ValidateLessonHasPDF(ctx context.Context, lessonID string) error {
	if err := u.requireLesson(ctx, lessonID); err != nil {
		return err
	}
	repo := u.repo

	lesson, err := repo.GetByID(ctx, lessonID)
	if err != nil {
		if errors.Is(err, memberclasserrors.ErrLessonNotFound) {
			return &memberclasserrors.MemberClassError{
				Code:    404,
				Message: "lesson not found",
			}
		}
		u.logger.Error(fmt.Sprintf("Error getting lesson %s: %v", lessonID, err))
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
func (u *pdfProcessorUseCase) GetLessonWithPDFAsset(ctx context.Context, lessonID string) (*lessons.Lesson, error) {
	if err := u.requireLesson(ctx, lessonID); err != nil {
		return nil, err
	}
	repo := u.repo

	lesson, err := repo.GetByIDWithPDFAsset(ctx, lessonID)
	if err != nil {
		if errors.Is(err, memberclasserrors.ErrLessonNotFound) {
			return nil, &memberclasserrors.MemberClassError{
				Code:    404,
				Message: "lesson not found",
			}
		}
		u.logger.Error(fmt.Sprintf("Error getting lesson %s: %v", lessonID, err))
		return nil, &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "error getting lesson",
		}
	}

	return lesson, nil
}

// GetPDFPagesByAssetID - Get PDF pages by asset ID (searches all databases)
func (u *pdfProcessorUseCase) GetPDFPagesByAssetID(ctx context.Context, assetID string) ([]*lessons.LessonPDFPage, error) {
	pages, err := u.repo.GetPDFPagesByAssetID(ctx, assetID)
	if err != nil || len(pages) == 0 {
		return []*lessons.LessonPDFPage{}, nil
	}
	return pages, nil
}
