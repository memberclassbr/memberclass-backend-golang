package video

import (
	"bytes"
	"io"
	"net/http"

	http2 "github.com/memberclass-backend-golang/internal/application/handlers/http"
	"github.com/memberclass-backend-golang/internal/domain/dto"
	"github.com/memberclass-backend-golang/internal/domain/memberclasserrors"
	"github.com/memberclass-backend-golang/internal/domain/ports"
	"github.com/memberclass-backend-golang/internal/domain/ports/bunny"
	"github.com/memberclass-backend-golang/internal/domain/ports/tenant"
)

type VideoHandler struct {
	useCaseGetTenantBunnyCredentials  tenant.TenantGetTenantBunnyCredentialsUseCase
	useCaseUploadVideoBunnyCdnUseCase bunny.UploadVideoBunnyCdnUseCase
	log                               ports.Logger
}

func NewVideoHandler(useCaseGetTenantBunnyCredentials tenant.TenantGetTenantBunnyCredentialsUseCase,
	useCaseUploadVideoBunnyCdnUseCase bunny.UploadVideoBunnyCdnUseCase,
	log ports.Logger) *VideoHandler {
	return &VideoHandler{
		useCaseGetTenantBunnyCredentials:  useCaseGetTenantBunnyCredentials,
		useCaseUploadVideoBunnyCdnUseCase: useCaseUploadVideoBunnyCdnUseCase,
		log:                               log,
	}
}

func (h *VideoHandler) UploadVideo(w http.ResponseWriter, r *http.Request) {

	err := r.ParseMultipartForm(0)
	if err != nil {
		h.log.Error("Failed to parse multipart form", "error", err)
		http2.WriteError(w, "Failed to parse form", http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		h.log.Error("File not found in request", "error", err)
		http2.WriteError(w, "File is required", http.StatusBadRequest)
		return
	}
	defer file.Close()

	tenantID := r.FormValue("tenantId")
	if tenantID == "" {
		http2.WriteError(w, "tenantId is required", http.StatusBadRequest)
		return
	}

	title := r.FormValue("title")
	if title == "" {
		title = header.Filename
		h.log.Debug("Title was empty, using filename", "filename", header.Filename)
	}

	var buf bytes.Buffer
	_, err = io.Copy(&buf, file)
	if err != nil {
		h.log.Error("Failed to read file", "error", err)
		http2.WriteError(w, "Failed to read file", http.StatusInternalServerError)
		return
	}
	fileBytes := buf.Bytes()

	h.log.Info("File received",
		"filename", header.Filename,
		"title", title,
		"size", len(fileBytes),
		"tenantID", tenantID)

	credentials, err := h.useCaseGetTenantBunnyCredentials.Execute(tenantID)
	if err != nil {
		h.log.Error("Failed to get tenant credentials", "error", err, "tenantID", tenantID)

		if memberClassErr, ok := err.(*memberclasserrors.MemberClassError); ok {
			http2.WriteError(w, memberClassErr.Message, memberClassErr.Code)
		} else {
			http2.WriteError(w, "Tenant not found", http.StatusNotFound)
		}
		return
	}

	bunnyParams := dto.BunnyParametersAccess{
		LibraryID:     credentials.BunnyLibraryID,
		LibraryApiKey: credentials.BunnyLibraryApiKey,
	}

	result, err := h.useCaseUploadVideoBunnyCdnUseCase.Execute(r.Context(), bunnyParams, fileBytes, title)
	if err != nil {
		h.log.Error("Upload failed", "error", err, "tenantID", tenantID)

		if memberClassErr, ok := err.(*memberclasserrors.MemberClassError); ok {
			http2.WriteError(w, memberClassErr.Message, memberClassErr.Code)
		} else {
			http2.WriteError(w, "Upload failed", http.StatusInternalServerError)
		}
		return
	}

	http2.WriteSuccess(w, result, http.StatusOK)
}
