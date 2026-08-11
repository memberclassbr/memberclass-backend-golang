package video

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/memberclass-backend-golang/internal/platform/bunny"
	"github.com/memberclass-backend-golang/internal/shared/httpx"
	"github.com/memberclass-backend-golang/internal/shared/memberclasserrors"
	"github.com/memberclass-backend-golang/internal/shared/utils"
)

const (
	// embedURLPrefix and embedURLParams build the player URL stored on the
	// lesson. The parameters are part of the stored URL, so changing them
	// changes existing content's playback behaviour.
	embedURLPrefix = "https://iframe.mediadelivery.net/embed/"
	embedURLParams = "?autoplay=false&loop=false&muted=false&preload=true&responsive=true"

	// socialCollection is the Bunny collection every upload from this endpoint
	// is filed under.
	socialCollection = "social"
)

// ---------- DTOs ----------

type uploadVideoResponse struct {
	OK       bool   `json:"ok"`
	MediaURL string `json:"mediaUrl"`
	GUID     string `json:"guid"`
	Title    string `json:"title"`
}

// ---------- 1. HTTP handler ----------

// UploadVideo handles `POST /api/v1/videos/upload`, a multipart form carrying
// the file, the tenantId and an optional title.
func (f *Feature) UploadVideo(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(0); err != nil {
		f.log.Error("Failed to parse multipart form", "error", err)
		httpx.WriteError(w, "Failed to parse form", http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		f.log.Error("File not found in request", "error", err)
		httpx.WriteError(w, "File is required", http.StatusBadRequest)
		return
	}
	defer file.Close()

	tenantID := r.FormValue("tenantId")
	if tenantID == "" {
		httpx.WriteError(w, "tenantId is required", http.StatusBadRequest)
		return
	}

	title := r.FormValue("title")
	if title == "" {
		title = header.Filename
		f.log.Debug("Title was empty, using filename", "filename", header.Filename)
	}

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, file); err != nil {
		f.log.Error("Failed to read file", "error", err)
		httpx.WriteError(w, "Failed to read file", http.StatusInternalServerError)
		return
	}
	fileBytes := buf.Bytes()

	f.log.Info("File received",
		"filename", header.Filename,
		"title", title,
		"size", len(fileBytes),
		"tenantID", tenantID)

	result, err := f.upload(r.Context(), tenantID, title, fileBytes)
	if err != nil {
		f.log.Error("Upload failed", "error", err, "tenantID", tenantID)
		writeUseCaseError(w, err)
		return
	}

	httpx.WriteSuccess(w, result, http.StatusOK)
}

func writeUseCaseError(w http.ResponseWriter, err error) {
	var mcErr *memberclasserrors.MemberClassError
	if errors.As(err, &mcErr) {
		httpx.WriteError(w, mcErr.Message, mcErr.Code)
		return
	}
	httpx.WriteError(w, "Upload failed", http.StatusInternalServerError)
}

// ---------- 2. Business rule ----------

func (f *Feature) upload(ctx context.Context, tenantID, title string, fileBytes []byte) (*uploadVideoResponse, error) {
	access, err := f.bunnyCredentials(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	collectionID := f.ensureSocialCollection(ctx, access)

	contentType := utils.DetectFileMimetype(fileBytes)
	f.log.Debug("File mimetype detected", "contentType", contentType)

	created, err := f.bunny.CreateVideo(ctx, bunny.CreateVideoRequest{
		Title:        title,
		CollectionID: collectionID,
	}, access)
	if err != nil {
		f.log.Error("Error creating video", "error", err, "title", title, "collectionID", collectionID)
		return nil, &memberclasserrors.MemberClassError{Code: 500, Message: "Error creating video"}
	}

	f.log.Info("Video created successfully", "guid", created.GUID)

	err = f.bunny.UploadVideo(ctx, bunny.UploadVideoRequest{
		GUID:        created.GUID,
		File:        fileBytes,
		ContentType: contentType,
	}, access)
	if err != nil {
		f.log.Error("Error uploading video file", "error", err, "guid", created.GUID)
		return nil, &memberclasserrors.MemberClassError{Code: 500, Message: "Error send video"}
	}

	mediaURL := embedURL(access.LibraryID, created.GUID)

	f.log.Info("Video upload process completed successfully",
		"guid", created.GUID, "mediaURL", mediaURL, "title", title)

	return &uploadVideoResponse{
		OK:       true,
		MediaURL: mediaURL,
		GUID:     created.GUID,
		Title:    title,
	}, nil
}

// ensureSocialCollection returns the id of the "social" collection, creating it
// if absent. A failure here is not fatal: the upload proceeds without a
// collection rather than failing outright.
func (f *Feature) ensureSocialCollection(ctx context.Context, access bunny.ParametersAccess) string {
	collections, err := f.bunny.GetCollections(ctx, access)
	if err != nil {
		f.log.Warn("Failed to get collections, proceeding without collection", "error", err)
		return ""
	}

	if collections != nil {
		for _, collection := range collections.Items {
			if collection.Name == socialCollection {
				return collection.GUID
			}
		}
	}

	f.log.Info("Social collection not found, creating new one")

	created, err := f.bunny.CreateCollection(ctx, bunny.CreateCollectionRequest{Name: socialCollection}, access)
	if err != nil {
		f.log.Warn("Failed to create social collection, proceeding without collection", "error", err)
		return ""
	}

	f.log.Info("Social collection created successfully", "guid", created.GUID)
	return created.GUID
}

func embedURL(libraryID, guid string) string {
	var b strings.Builder
	b.WriteString(embedURLPrefix)
	b.WriteString(libraryID)
	b.WriteString("/")
	b.WriteString(guid)
	b.WriteString(embedURLParams)
	return b.String()
}

// ---------- 3. SQL ----------

// sqlTenantBunnyCredentials reads the per-tenant Bunny library credentials.
// Each tenant owns a separate Bunny library, so uploads cannot cross tenants.
const sqlTenantBunnyCredentials = `
	SELECT "bunnyLibraryId", "bunnyLibraryApiKey"
	FROM "Tenant"
	WHERE id = $1
`

func (f *Feature) bunnyCredentials(ctx context.Context, tenantID string) (bunny.ParametersAccess, error) {
	if tenantID == "" {
		return bunny.ParametersAccess{}, memberclasserrors.ErrTenantIDEmpty
	}

	var libraryID, apiKey sql.NullString
	err := f.db.QueryRowContext(ctx, sqlTenantBunnyCredentials, tenantID).Scan(&libraryID, &apiKey)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return bunny.ParametersAccess{}, memberclasserrors.ErrTenantNotFound
	case err != nil:
		f.log.Error("Error finding tenant: " + err.Error())
		return bunny.ParametersAccess{}, &memberclasserrors.MemberClassError{Code: 500, Message: "error finding tenant"}
	}

	return bunny.ParametersAccess{
		LibraryID:     libraryID.String,
		LibraryApiKey: apiKey.String,
	}, nil
}
