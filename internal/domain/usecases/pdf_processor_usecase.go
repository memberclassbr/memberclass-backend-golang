package usecases

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/memberclass-backend-golang/internal/domain/dto"
	"github.com/memberclass-backend-golang/internal/domain/entities"
	"github.com/memberclass-backend-golang/internal/domain/memberclasserrors"
	"github.com/memberclass-backend-golang/internal/domain/ports"
)

type pdfProcessorUseCase struct {
	lessonRepo     ports.LessonRepository
	pdfService     ports.PdfProcessService
	storageService ports.Storage
	logger         ports.Logger
	mu             sync.RWMutex
}

type ProcessingJob struct {
	LessonID string
	Lesson   *entities.Lesson
}

type ProcessingResult struct {
	LessonID string
	Result   *dto.ProcessResult
	Error    error
}

func NewPdfProcessorUseCase(
	lessonRepo ports.LessonRepository,
	pdfService ports.PdfProcessService,
	storageService ports.Storage,
	logger ports.Logger,
) ports.PdfProcessorUseCase {
	return &pdfProcessorUseCase{
		lessonRepo:     lessonRepo,
		pdfService:     pdfService,
		storageService: storageService,
		logger:         logger,
	}
}

// ProcessLesson - Process a single lesson PDF
func (u *pdfProcessorUseCase) ProcessLesson(ctx context.Context, lessonID string) (*dto.ProcessResult, error) {
	// 1. Get lesson with PDF asset
	lesson, err := u.GetLessonWithPDFAsset(ctx, lessonID)
	if err != nil {
		return nil, err
	}

	// 2. Validate lesson has PDF
	err = u.ValidateLessonHasPDF(ctx, lessonID)
	if err != nil {
		return nil, err
	}

	// 3. Create or update PDF asset
	asset, err := u.CreateOrUpdatePDFAsset(ctx, lessonID, *lesson.MediaURL)
	if err != nil {
		u.logger.Error(fmt.Sprintf("Error creating/updating PDF asset for lesson %s: %v", lessonID, err))
		return nil, &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "error creating PDF asset",
		}
	}

	// 4. Process PDF to images using the complete flow
	images, err := u.ConvertPdfToImages(*lesson.MediaURL)
	if err != nil {
		errorMsg := err.Error()
		u.lessonRepo.UpdatePDFAssetStatus(ctx, asset.ID, "failed", nil, &errorMsg)
		u.logger.Error(fmt.Sprintf("Error converting PDF to images for lesson %s: %v", lessonID, err))
		return nil, &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "error converting PDF to images",
		}
	}

	// 5. Save pages directly
	processedPages, err := u.SavePagesDirectly(ctx, asset.ID, lessonID, images)
	totalImages := len(images)
	if err != nil {
		errorMsg := err.Error()
		u.lessonRepo.UpdatePDFAssetStatus(ctx, asset.ID, "partial", &totalImages, &errorMsg)
		u.logger.Error(fmt.Sprintf("Error saving pages for lesson %s: %v", lessonID, err))
		return nil, &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "error saving pages",
		}
	}

	// 6. Update final status
	status := "done"
	if processedPages < len(images) {
		status = "partial"
	}

	err = u.lessonRepo.UpdatePDFAssetStatus(ctx, asset.ID, status, &totalImages, nil)
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

// ProcessAllPendingLessons - Process all pending PDF lessons
func (u *pdfProcessorUseCase) ProcessAllPendingLessons(ctx context.Context, limit int) (*dto.BatchProcessResult, error) {
	lessons, err := u.lessonRepo.GetPendingPDFLessons(ctx, limit)
	if err != nil {
		u.logger.Error(fmt.Sprintf("Error getting pending PDF lessons: %v", err))
		return nil, &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "error getting pending lessons",
		}
	}

	if len(lessons) == 0 {
		return &dto.BatchProcessResult{
			Processed: 0,
			Total:     0,
			Results:   []dto.ProcessResult{},
		}, nil
	}

	// Process lessons concurrently using worker pool
	const maxWorkers = 5
	jobChan := make(chan ProcessingJob, len(lessons))
	resultChan := make(chan ProcessingResult, len(lessons))

	var wg sync.WaitGroup
	var mu sync.Mutex
	results := make([]dto.ProcessResult, 0, len(lessons))
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
		for _, lesson := range lessons {
			select {
			case jobChan <- ProcessingJob{LessonID: lesson.ID, Lesson: lesson}:
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
		Total:     len(lessons),
		Results:   results,
	}, nil
}

// RetryFailedAssets - Retry processing failed PDF assets
func (u *pdfProcessorUseCase) RetryFailedAssets(ctx context.Context) error {
	failedAssets, err := u.lessonRepo.GetFailedPDFAssets(ctx)
	if err != nil {
		u.logger.Error(fmt.Sprintf("Error getting failed PDF assets: %v", err))
		return &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "error getting failed assets",
		}
	}

	if len(failedAssets) == 0 {
		return nil
	}

	// Retry assets concurrently using worker pool
	const maxWorkers = 3
	jobChan := make(chan entities.LessonPDFAsset, len(failedAssets))
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
					_, err := u.ProcessLesson(ctx, asset.LessonID)
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
			u.logger.Error(fmt.Sprintf("Failed to retry asset: %v", err))
		}
	}

	return nil
}

// CleanupOrphanedPages - Clean up orphaned PDF pages
func (u *pdfProcessorUseCase) CleanupOrphanedPages(ctx context.Context) error {
	failedAssets, err := u.lessonRepo.GetFailedPDFAssets(ctx)
	if err != nil {
		u.logger.Error(fmt.Sprintf("Error getting failed PDF assets: %v", err))
		return &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "error getting failed assets",
		}
	}

	if len(failedAssets) == 0 {
		return nil
	}

	// Clean up pages concurrently using worker pool
	const maxWorkers = 3
	jobChan := make(chan entities.LessonPDFAsset, len(failedAssets))
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
					err := u.cleanupAssetPages(ctx, asset)
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

// cleanupAssetPages - Clean up pages for a single asset
func (u *pdfProcessorUseCase) cleanupAssetPages(ctx context.Context, asset entities.LessonPDFAsset) error {
	pages, err := u.lessonRepo.GetPDFPagesByAssetID(ctx, asset.ID)
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
					err := u.lessonRepo.DeletePDFPage(ctx, job.pageID)
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
	// 1. Get lesson
	lesson, err := u.GetLessonWithPDFAsset(ctx, lessonID)
	if err != nil {
		return err
	}

	// 2. Validate lesson has PDF
	err = u.ValidateLessonHasPDF(ctx, lessonID)
	if err != nil {
		return err
	}

	// 3. Delete existing PDF asset and pages if exists
	if lesson.PDFAsset != nil {
		// Delete all pages
		err = u.lessonRepo.DeletePDFPagesByAssetID(ctx, lesson.PDFAsset.ID)
		if err != nil {
			u.logger.Error(fmt.Sprintf("Error deleting pages for asset %s: %v", lesson.PDFAsset.ID, err))
		}

		// Reset asset status
		err = u.lessonRepo.UpdatePDFAssetStatus(ctx, lesson.PDFAsset.ID, "pending", nil, nil)
		if err != nil {
			u.logger.Error(fmt.Sprintf("Error resetting asset %s: %v", lesson.PDFAsset.ID, err))
		}
	} else {
		// Create new asset
		asset := &entities.LessonPDFAsset{
			LessonID:     lessonID,
			SourcePDFURL: *lesson.MediaURL,
			Status:       "pending",
		}
		err = u.lessonRepo.CreatePDFAsset(ctx, asset)
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
	// 1. Get authentication token
	token, err := u.pdfService.GetToken()
	if err != nil {
		return nil, fmt.Errorf("failed to get token: %w", err)
	}

	// 2. Create task
	task, err := u.pdfService.CreateTask(token)
	if err != nil {
		return nil, fmt.Errorf("failed to create task: %w", err)
	}

	// 3. Add file
	serverFilename, err := u.pdfService.AddFile(token, task.Task, pdfURL, task.Server)
	if err != nil {
		return nil, fmt.Errorf("failed to add file: %w", err)
	}

	// 4. Process task
	err = u.pdfService.ProcessTask(token, task.Task, serverFilename, task.Server)
	if err != nil {
		return nil, fmt.Errorf("failed to process task: %w", err)
	}

	// 5. Download result
	zipData, err := u.pdfService.DownloadTask(token, task.Task, task.Server)
	if err != nil {
		return nil, fmt.Errorf("failed to download task: %w", err)
	}

	// 6. Extract images
	images, err := u.pdfService.ExtractImagesFromZip(zipData)
	if err != nil {
		return nil, fmt.Errorf("failed to extract images: %w", err)
	}

	return images, nil
}

// CreateOrUpdatePDFAsset - Create or update PDF asset
func (u *pdfProcessorUseCase) CreateOrUpdatePDFAsset(ctx context.Context, lessonID, pdfURL string) (*entities.LessonPDFAsset, error) {
	asset, err := u.lessonRepo.GetPDFAssetByLessonID(ctx, lessonID)
	if err != nil && !errors.Is(err, memberclasserrors.ErrPDFAssetNotFound) {
		return nil, err
	}

	if asset == nil {
		// Create new asset with generated ID
		asset = &entities.LessonPDFAsset{
			ID:           uuid.New().String(),
			LessonID:     lessonID,
			SourcePDFURL: pdfURL,
			Status:       "processing",
		}
		err = u.lessonRepo.CreatePDFAsset(ctx, asset)
	} else {
		// Update existing asset
		asset.Status = "processing"
		asset.Error = nil
		err = u.lessonRepo.UpdatePDFAsset(ctx, asset)
	}

	return asset, err
}

// saveSinglePage - Save a single page (thread-safe)
func (u *pdfProcessorUseCase) saveSinglePage(ctx context.Context, assetID string, pageNumber int, imageBase64 string) (bool, error) {
	existingPage, err := u.lessonRepo.GetPDFPageByAssetAndNumber(ctx, assetID, pageNumber)
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

	// 4. Upload to DigitalOcean Spaces
	u.logger.Info(fmt.Sprintf("Uploading page %d to storage for asset %s", pageNumber, assetID))
	imageURL, err := u.storageService.Upload(ctx, imageData, filename, "image/jpeg")
	if err != nil {
		u.logger.Error(fmt.Sprintf("Failed to upload page %d to storage for asset %s: %v", pageNumber, assetID, err))
		return false, fmt.Errorf("failed to upload image to storage: %w", err)
	}

	u.logger.Info(fmt.Sprintf("Successfully uploaded page %d to storage: %s", pageNumber, imageURL))

	// 5. Save storage URL to database
	page := &entities.LessonPDFPage{
		ID:         uuid.New().String(),
		AssetID:    assetID,
		PageNumber: pageNumber,
		ImageURL:   imageURL, // DigitalOcean Spaces URL
	}

	err = u.lessonRepo.CreatePDFPage(ctx, page)
	if err != nil {
		u.logger.Error(fmt.Sprintf("Failed to save page %d to database for asset %s: %v", pageNumber, assetID, err))
		return false, fmt.Errorf("failed to save page to database: %w", err)
	}

	return true, nil
}

// SavePagesDirectly - Save pages directly without upload service
func (u *pdfProcessorUseCase) SavePagesDirectly(ctx context.Context, assetID, lessonID string, images []string) (int, error) {
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
					success, err := u.saveSinglePage(ctx, assetID, job.pageNumber, job.imageBase64)
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

	return processedPages, nil
}

// ValidateLessonHasPDF - Validate that lesson has a PDF media URL
func (u *pdfProcessorUseCase) ValidateLessonHasPDF(ctx context.Context, lessonID string) error {
	lesson, err := u.lessonRepo.GetByID(ctx, lessonID)
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

// GetLessonWithPDFAsset - Get lesson with PDF asset relationship
func (u *pdfProcessorUseCase) GetLessonWithPDFAsset(ctx context.Context, lessonID string) (*entities.Lesson, error) {
	lesson, err := u.lessonRepo.GetByIDWithPDFAsset(ctx, lessonID)
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
