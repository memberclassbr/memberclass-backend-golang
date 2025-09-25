package usecases

import (
	"context"
	"fmt"
	"strings"

	"github.com/memberclass-backend-golang/internal/domain/dto"
	"github.com/memberclass-backend-golang/internal/domain/memberclasserrors"
	"github.com/memberclass-backend-golang/internal/domain/ports"
	"github.com/memberclass-backend-golang/internal/domain/utils"
)

const (
	mediaURL           = "https://iframe.mediadelivery.net/embed/"
	mediaURLParameters = "?autoplay=false&loop=false&muted=false&preload=true&responsive=true"
	collectionSocial   = "social"
)

type UploadVideoBunnyCdnUseCase struct {
	bunnyService ports.BunnyService
	log          ports.Logger
}

func (u *UploadVideoBunnyCdnUseCase) Execute(ctx context.Context, bunnyParams dto.BunnyParametersAccess, fileBytes []byte, title string) (*dto.UploadVideoResponse, error) {
	u.log.Info("Starting video upload process", 
		"libraryID", bunnyParams.LibraryID, 
		"title", title, 
		"fileSize", len(fileBytes))

	collectionID := u.ensureSocialCollection(ctx, bunnyParams)
	u.log.Debug("Collection ID resolved", "collectionID", collectionID)

	createVideoReq := dto.CreateVideoRequest{
		Title:        title,
		CollectionID: collectionID,
	}

	contentType := utils.DetectFileMimetype(fileBytes)
	u.log.Debug("File mimetype detected", "contentType", contentType)

	u.log.Debug("Creating video in Bunny", "title", title, "collectionID", collectionID)
	createVideoResp, err := u.bunnyService.CreateVideo(ctx, createVideoReq, bunnyParams)
	if err != nil {
		u.log.Error("Error creating video", "error", err, "title", title, "collectionID", collectionID)
		return nil, &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "Error creating video",
		}
	}

	u.log.Info("Video created successfully", "guid", createVideoResp.GUID)

	uploadReq := dto.UploadVideoRequest{
		GUID:        createVideoResp.GUID,
		File:        fileBytes,
		ContentType: contentType,
	}

	u.log.Debug("Starting file upload", "guid", createVideoResp.GUID, "fileSize", len(fileBytes))
	err = u.bunnyService.UploadVideo(ctx, uploadReq, bunnyParams)
	if err != nil {
		u.log.Error("Error uploading video file", "error", err, "guid", createVideoResp.GUID)
		return nil, &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "Error send video",
		}
	}

	u.log.Info("File upload completed successfully", "guid", createVideoResp.GUID)

	urlMedia := u.generatedMediaUrl(ctx, bunnyParams.LibraryID, uploadReq.GUID)
	u.log.Debug("Media URL generated", "mediaURL", urlMedia)

	u.log.Info("Video upload process completed successfully", 
		"guid", uploadReq.GUID, 
		"mediaURL", urlMedia,
		"title", title)

	return &dto.UploadVideoResponse{
		OK:       true,
		MediaURL: urlMedia,
		GUID:     uploadReq.GUID,
		Title:    title,
	}, nil

}

func (u *UploadVideoBunnyCdnUseCase) ensureSocialCollection(ctx context.Context, bunnyParams dto.BunnyParametersAccess) string {
	u.log.Debug("Ensuring social collection exists")

	collections, err := u.bunnyService.GetCollections(ctx, bunnyParams)
	if err != nil {
		u.log.Warn("Failed to get collections, proceeding without collection", "error", err)
		return ""
	}

	u.log.Debug("Collections retrieved", "count", len(*collections))

	if collections != nil {
		for _, collection := range *collections {
			if collection.Name == collectionSocial {
				u.log.Debug("Social collection found", "guid", collection.GUID)
				return collection.GUID
			}
		}
	}

	u.log.Info("Social collection not found, creating new one")

	createCollectionReq := dto.CreateCollectionRequest{
		Name: collectionSocial,
	}

	createResp, err := u.bunnyService.CreateCollection(ctx, createCollectionReq, bunnyParams)
	if err != nil {
		u.log.Warn("Failed to create social collection, proceeding without collection", "error", err)
		return ""
	}

	u.log.Info("Social collection created successfully", "guid", createResp.GUID)
	return createResp.GUID
}

func (u *UploadVideoBunnyCdnUseCase) generatedMediaUrl(ctx context.Context, libraryID, guid string) string {
	var builder strings.Builder

	builder.WriteString(mediaURL)
	builder.WriteString(libraryID)
	builder.WriteString("/")
	builder.WriteString(guid)
	builder.WriteString(mediaURLParameters)

	return builder.String()
}

func NewUploadVideoBunnyCdnUseCase(bunnyService ports.BunnyService, log ports.Logger) ports.UploadVideoBunnyCdnUseCase {
	return &UploadVideoBunnyCdnUseCase{
		bunnyService: bunnyService,
		log:          log,
	}
}
