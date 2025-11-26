package lesson

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/memberclass-backend-golang/internal/domain/entities"
	"github.com/memberclass-backend-golang/internal/domain/memberclasserrors"
	"github.com/memberclass-backend-golang/internal/domain/ports"
)

type LessonRepository struct {
	db  *sql.DB
	log ports.Logger
}

func NewLessonRepository(db *sql.DB, log ports.Logger) ports.LessonRepository {
	return &LessonRepository{
		db:  db,
		log: log,
	}
}

// GetByID - Get lesson by ID
func (l *LessonRepository) GetByID(ctx context.Context, id string) (*entities.Lesson, error) {
	query := `SELECT id, "createdAt", "updatedAt", access, "referenceAccess", type, slug, name, 
		published, "order", "mediaUrl", "fullHdStatus", "fullHdUrl", "fullHdRetries", 
		thumbnail, content, "moduleId", "createdBy", "showDescriptionToggle", 
		"bannersTitle", "transcriptionCompleted" 
		FROM "Lesson" WHERE id = $1`

	var lesson entities.Lesson
	var mediaURL sql.NullString

	err := l.db.QueryRowContext(ctx, query, id).Scan(
		&lesson.ID,
		&lesson.CreatedAt,
		&lesson.UpdatedAt,
		&lesson.Access,
		&lesson.ReferenceAccess,
		&lesson.Type,
		&lesson.Slug,
		&lesson.Name,
		&lesson.Published,
		&lesson.Order,
		&mediaURL,
		&lesson.FullHDStatus,
		&lesson.FullHDURL,
		&lesson.FullHDRetries,
		&lesson.Thumbnail,
		&lesson.Content,
		&lesson.ModuleID,
		&lesson.CreatedBy,
		&lesson.ShowDescriptionToggle,
		&lesson.BannersTitle,
		&lesson.TranscriptionCompleted,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, memberclasserrors.ErrLessonNotFound
		}
		l.log.Error(err.Error())
		return nil, &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "error finding lesson",
		}
	}

	if mediaURL.Valid {
		lesson.MediaURL = &mediaURL.String
	}

	return &lesson, nil
}

// GetByIDWithPDFAsset - Get lesson with PDF asset relationship
func (l *LessonRepository) GetByIDWithPDFAsset(ctx context.Context, id string) (*entities.Lesson, error) {
	query := `SELECT l.id, l."createdAt", l."updatedAt", l.access, l."referenceAccess", l.type, 
		l.slug, l.name, l.published, l."order", l."mediaUrl", l."fullHdStatus", 
		l."fullHdUrl", l."fullHdRetries", l.thumbnail, l.content, l."moduleId", 
		l."createdBy", l."showDescriptionToggle", l."bannersTitle", l."transcriptionCompleted",
		p.id, p."lessonId", p."sourcePdfUrl", p."totalPages", p.status, p.error, 
		p."createdAt", p."updatedAt"
		FROM "Lesson" l
		LEFT JOIN "LessonPdfAsset" p ON l.id = p."lessonId"
		WHERE l.id = $1`

	var lesson entities.Lesson
	var mediaURL sql.NullString
	var pdfAssetID, pdfLessonID, pdfSourceURL, pdfStatus, pdfError sql.NullString
	var pdfTotalPages sql.NullInt32
	var pdfCreatedAt, pdfUpdatedAt sql.NullTime

	err := l.db.QueryRowContext(ctx, query, id).Scan(
		&lesson.ID,
		&lesson.CreatedAt,
		&lesson.UpdatedAt,
		&lesson.Access,
		&lesson.ReferenceAccess,
		&lesson.Type,
		&lesson.Slug,
		&lesson.Name,
		&lesson.Published,
		&lesson.Order,
		&mediaURL,
		&lesson.FullHDStatus,
		&lesson.FullHDURL,
		&lesson.FullHDRetries,
		&lesson.Thumbnail,
		&lesson.Content,
		&lesson.ModuleID,
		&lesson.CreatedBy,
		&lesson.ShowDescriptionToggle,
		&lesson.BannersTitle,
		&lesson.TranscriptionCompleted,
		&pdfAssetID,
		&pdfLessonID,
		&pdfSourceURL,
		&pdfTotalPages,
		&pdfStatus,
		&pdfError,
		&pdfCreatedAt,
		&pdfUpdatedAt,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, memberclasserrors.ErrLessonNotFound
		}
		l.log.Error(err.Error())
		return nil, &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "error finding lesson with PDF asset",
		}
	}

	if mediaURL.Valid {
		lesson.MediaURL = &mediaURL.String
	}

	// Build PDF asset if exists
	if pdfAssetID.Valid {
		asset := &entities.LessonPDFAsset{
			ID:           pdfAssetID.String,
			LessonID:     pdfLessonID.String,
			SourcePDFURL: pdfSourceURL.String,
			Status:       pdfStatus.String,
			CreatedAt:    pdfCreatedAt.Time,
			UpdatedAt:    pdfUpdatedAt.Time,
		}

		if pdfTotalPages.Valid {
			totalPages := int(pdfTotalPages.Int32)
			asset.TotalPages = &totalPages
		}

		if pdfError.Valid {
			asset.Error = &pdfError.String
		}

		lesson.PDFAsset = asset
	}

	return &lesson, nil
}

// GetPendingPDFLessons - Get lessons that need PDF processing
func (l *LessonRepository) GetPendingPDFLessons(ctx context.Context, limit int) ([]*entities.Lesson, error) {
	query := `SELECT id, "createdAt", "updatedAt", access, "referenceAccess", type, 
		slug, name, published, "order", "mediaUrl", "fullHdStatus", 
		"fullHdUrl", "fullHdRetries", thumbnail, content, "moduleId", 
		"createdBy", "showDescriptionToggle", "bannersTitle", "transcriptionCompleted"
		FROM "Lesson" 
		WHERE "mediaUrl" LIKE '%.pdf'
		  AND id NOT IN (
		      SELECT DISTINCT "lessonId" 
		      FROM "LessonPdfAsset" 
		      WHERE "lessonId" IS NOT NULL
		  )
		ORDER BY "createdAt" ASC
		LIMIT $1`

	rows, err := l.db.QueryContext(ctx, query, limit)
	if err != nil {
		l.log.Error(err.Error())
		return nil, &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "error getting pending PDF lessons",
		}
	}
	defer rows.Close()

	var lessons []*entities.Lesson
	for rows.Next() {
		var lesson entities.Lesson
		var mediaURL sql.NullString

		err := rows.Scan(
			&lesson.ID,
			&lesson.CreatedAt,
			&lesson.UpdatedAt,
			&lesson.Access,
			&lesson.ReferenceAccess,
			&lesson.Type,
			&lesson.Slug,
			&lesson.Name,
			&lesson.Published,
			&lesson.Order,
			&mediaURL,
			&lesson.FullHDStatus,
			&lesson.FullHDURL,
			&lesson.FullHDRetries,
			&lesson.Thumbnail,
			&lesson.Content,
			&lesson.ModuleID,
			&lesson.CreatedBy,
			&lesson.ShowDescriptionToggle,
			&lesson.BannersTitle,
			&lesson.TranscriptionCompleted,
		)

		if err != nil {
			l.log.Error(err.Error())
			return nil, &memberclasserrors.MemberClassError{
				Code:    500,
				Message: "error scanning lesson",
			}
		}

		if mediaURL.Valid {
			lesson.MediaURL = &mediaURL.String
		}

		lessons = append(lessons, &lesson)
	}

	if err = rows.Err(); err != nil {
		l.log.Error(err.Error())
		return nil, &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "error iterating lessons",
		}
	}

	return lessons, nil
}

// Update - Update lesson
func (l *LessonRepository) Update(ctx context.Context, lesson *entities.Lesson) error {
	query := `UPDATE "Lesson" 
		SET "updatedAt" = $1, access = $2, "referenceAccess" = $3, type = $4, slug = $5, 
		    name = $6, published = $7, "order" = $8, "mediaUrl" = $9, "fullHdStatus" = $10, 
		    "fullHdUrl" = $11, "fullHdRetries" = $12, thumbnail = $13, content = $14, 
		    "moduleId" = $15, "createdBy" = $16, "showDescriptionToggle" = $17, 
		    "bannersTitle" = $18, "transcriptionCompleted" = $19
		WHERE id = $20`

	var mediaURL interface{}
	if lesson.MediaURL != nil {
		mediaURL = *lesson.MediaURL
	}

	_, err := l.db.ExecContext(ctx, query,
		time.Now(), lesson.Access, lesson.ReferenceAccess, lesson.Type, lesson.Slug,
		lesson.Name, lesson.Published, lesson.Order, mediaURL, lesson.FullHDStatus,
		lesson.FullHDURL, lesson.FullHDRetries, lesson.Thumbnail, lesson.Content,
		lesson.ModuleID, lesson.CreatedBy, lesson.ShowDescriptionToggle,
		lesson.BannersTitle, lesson.TranscriptionCompleted, lesson.ID,
	)

	if err != nil {
		l.log.Error(err.Error())
		return &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "error updating lesson",
		}
	}

	return nil
}

// PDF Asset operations

// GetPDFAssetByLessonID - Get PDF asset by lesson ID
func (l *LessonRepository) GetPDFAssetByLessonID(ctx context.Context, lessonID string) (*entities.LessonPDFAsset, error) {
	query := `SELECT id, "lessonId", "sourcePdfUrl", "totalPages", status, error, "createdAt", "updatedAt"
		FROM "LessonPdfAsset" 
		WHERE "lessonId" = $1`

	var asset entities.LessonPDFAsset
	var totalPages sql.NullInt32
	var error sql.NullString

	err := l.db.QueryRowContext(ctx, query, lessonID).Scan(
		&asset.ID,
		&asset.LessonID,
		&asset.SourcePDFURL,
		&totalPages,
		&asset.Status,
		&error,
		&asset.CreatedAt,
		&asset.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, memberclasserrors.ErrPDFAssetNotFound
		}
		l.log.Error(err.Error())
		return nil, &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "error finding PDF asset",
		}
	}

	if totalPages.Valid {
		totalPagesInt := int(totalPages.Int32)
		asset.TotalPages = &totalPagesInt
	}

	if error.Valid {
		asset.Error = &error.String
	}

	return &asset, nil
}

// CreatePDFAsset - Create new PDF asset
func (l *LessonRepository) CreatePDFAsset(ctx context.Context, asset *entities.LessonPDFAsset) error {
	query := `INSERT INTO "LessonPdfAsset" (id, "lessonId", "sourcePdfUrl", "totalPages", status, error, "createdAt", "updatedAt")
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`

	var totalPages interface{}
	if asset.TotalPages != nil {
		totalPages = *asset.TotalPages
	}

	var error interface{}
	if asset.Error != nil {
		error = *asset.Error
	}

	now := time.Now()
	_, err := l.db.ExecContext(ctx, query,
		asset.ID, asset.LessonID, asset.SourcePDFURL, totalPages,
		asset.Status, error, now, now,
	)

	if err != nil {
		l.log.Error(err.Error())
		return &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "error creating PDF asset",
		}
	}

	asset.CreatedAt = now
	asset.UpdatedAt = now
	return nil
}

// UpdatePDFAsset - Update PDF asset
func (l *LessonRepository) UpdatePDFAsset(ctx context.Context, asset *entities.LessonPDFAsset) error {
	query := `UPDATE "LessonPdfAsset" 
		SET "sourcePdfUrl" = $1, "totalPages" = $2, status = $3, error = $4, "updatedAt" = $5
		WHERE id = $6`

	var totalPages interface{}
	if asset.TotalPages != nil {
		totalPages = *asset.TotalPages
	}

	var error interface{}
	if asset.Error != nil {
		error = *asset.Error
	}

	_, err := l.db.ExecContext(ctx, query,
		asset.SourcePDFURL, totalPages, asset.Status, error, time.Now(), asset.ID,
	)

	if err != nil {
		l.log.Error(err.Error())
		return &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "error updating PDF asset",
		}
	}

	return nil
}

// UpdatePDFAssetStatus - Update only status, total_pages and error
func (l *LessonRepository) UpdatePDFAssetStatus(ctx context.Context, assetID, status string, totalPages *int, errorMsg *string) error {
	query := `UPDATE "LessonPdfAsset" 
		SET status = $1, "totalPages" = $2, error = $3, "updatedAt" = $4
		WHERE id = $5`

	var totalPagesVal interface{}
	if totalPages != nil {
		totalPagesVal = *totalPages
	}

	var errorVal interface{}
	if errorMsg != nil {
		errorVal = *errorMsg
	}

	_, err := l.db.ExecContext(ctx, query,
		status, totalPagesVal, errorVal, time.Now(), assetID,
	)

	if err != nil {
		l.log.Error(err.Error())
		return &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "error updating PDF asset status",
		}
	}

	return nil
}

// GetFailedPDFAssets - Get failed PDF assets for retry
func (l *LessonRepository) GetFailedPDFAssets(ctx context.Context) ([]*entities.LessonPDFAsset, error) {
	query := `SELECT id, "lessonId", "sourcePdfUrl", "totalPages", status, error, "createdAt", "updatedAt"
		FROM "LessonPdfAsset" 
		WHERE status IN ('failed', 'partial')
		ORDER BY "updatedAt" ASC`

	rows, err := l.db.QueryContext(ctx, query)
	if err != nil {
		l.log.Error(err.Error())
		return nil, &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "error getting failed PDF assets",
		}
	}
	defer rows.Close()

	var assets []*entities.LessonPDFAsset
	for rows.Next() {
		var asset entities.LessonPDFAsset
		var totalPages sql.NullInt32
		var error sql.NullString

		err := rows.Scan(
			&asset.ID,
			&asset.LessonID,
			&asset.SourcePDFURL,
			&totalPages,
			&asset.Status,
			&error,
			&asset.CreatedAt,
			&asset.UpdatedAt,
		)

		if err != nil {
			l.log.Error(err.Error())
			return nil, &memberclasserrors.MemberClassError{
				Code:    500,
				Message: "error scanning PDF asset",
			}
		}

		if totalPages.Valid {
			totalPagesInt := int(totalPages.Int32)
			asset.TotalPages = &totalPagesInt
		}

		if error.Valid {
			asset.Error = &error.String
		}

		assets = append(assets, &asset)
	}

	if err = rows.Err(); err != nil {
		l.log.Error(err.Error())
		return nil, &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "error iterating PDF assets",
		}
	}

	return assets, nil
}

// PDF Page operations

// CreatePDFPage - Create new PDF page
func (l *LessonRepository) CreatePDFPage(ctx context.Context, page *entities.LessonPDFPage) error {
	query := `INSERT INTO "LessonPdfPage" (id, "assetId", "pageNumber", "imageUrl", width, height, "createdAt", "updatedAt")
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`

	var width, height interface{}
	if page.Width != nil {
		width = *page.Width
	}
	if page.Height != nil {
		height = *page.Height
	}

	now := time.Now()
	_, err := l.db.ExecContext(ctx, query,
		page.ID, page.AssetID, page.PageNumber, page.ImageURL, width, height, now, now,
	)

	if err != nil {
		l.log.Error(err.Error())
		return &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "error creating PDF page",
		}
	}

	page.CreatedAt = now
	page.UpdatedAt = now
	return nil
}

// GetPDFPageByAssetAndNumber - Get PDF page by asset ID and page number
func (l *LessonRepository) GetPDFPageByAssetAndNumber(ctx context.Context, assetID string, pageNumber int) (*entities.LessonPDFPage, error) {
	query := `SELECT id, "assetId", "pageNumber", "imageUrl", width, height, "createdAt", "updatedAt"
		FROM "LessonPdfPage" 
		WHERE "assetId" = $1 AND "pageNumber" = $2`

	var page entities.LessonPDFPage
	var width, height sql.NullInt32

	err := l.db.QueryRowContext(ctx, query, assetID, pageNumber).Scan(
		&page.ID,
		&page.AssetID,
		&page.PageNumber,
		&page.ImageURL,
		&width,
		&height,
		&page.CreatedAt,
		&page.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, memberclasserrors.ErrPDFPageNotFound
		}
		l.log.Error(err.Error())
		return nil, &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "error finding PDF page",
		}
	}

	if width.Valid {
		widthInt := int(width.Int32)
		page.Width = &widthInt
	}

	if height.Valid {
		heightInt := int(height.Int32)
		page.Height = &heightInt
	}

	return &page, nil
}

// GetPDFPagesByAssetID - Get all PDF pages by asset ID
func (l *LessonRepository) GetPDFPagesByAssetID(ctx context.Context, assetID string) ([]*entities.LessonPDFPage, error) {
	query := `SELECT id, "assetId", "pageNumber", "imageUrl", width, height, "createdAt", "updatedAt"
		FROM "LessonPdfPage" 
		WHERE "assetId" = $1
		ORDER BY "pageNumber" ASC`

	rows, err := l.db.QueryContext(ctx, query, assetID)
	if err != nil {
		l.log.Error(err.Error())
		return nil, &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "error getting PDF pages",
		}
	}
	defer rows.Close()

	var pages []*entities.LessonPDFPage
	for rows.Next() {
		var page entities.LessonPDFPage
		var width, height sql.NullInt32

		err := rows.Scan(
			&page.ID,
			&page.AssetID,
			&page.PageNumber,
			&page.ImageURL,
			&width,
			&height,
			&page.CreatedAt,
			&page.UpdatedAt,
		)

		if err != nil {
			l.log.Error(err.Error())
			return nil, &memberclasserrors.MemberClassError{
				Code:    500,
				Message: "error scanning PDF page",
			}
		}

		if width.Valid {
			widthInt := int(width.Int32)
			page.Width = &widthInt
		}

		if height.Valid {
			heightInt := int(height.Int32)
			page.Height = &heightInt
		}

		pages = append(pages, &page)
	}

	if err = rows.Err(); err != nil {
		l.log.Error(err.Error())
		return nil, &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "error iterating PDF pages",
		}
	}

	return pages, nil
}

// DeletePDFPage - Delete PDF page by ID
func (l *LessonRepository) DeletePDFPage(ctx context.Context, pageID string) error {
	query := `DELETE FROM "LessonPdfPage" WHERE id = $1`

	_, err := l.db.ExecContext(ctx, query, pageID)
	if err != nil {
		l.log.Error(err.Error())
		return &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "error deleting PDF page",
		}
	}

	return nil
}

// DeletePDFPagesByAssetID - Delete all PDF pages by asset ID
func (l *LessonRepository) DeletePDFPagesByAssetID(ctx context.Context, assetID string) error {
	query := `DELETE FROM "LessonPdfPage" WHERE "assetId" = $1`

	_, err := l.db.ExecContext(ctx, query, assetID)
	if err != nil {
		l.log.Error(err.Error())
		return &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "error deleting PDF pages by asset ID",
		}
	}

	return nil
}
