package ports

import (
	"context"

	"github.com/memberclass-backend-golang/internal/domain/dto"
)

type BunnyService interface {
	CreateCollection(ctx context.Context, createCollectionRequest dto.CreateCollectionRequest, bunnyParametersAccess dto.BunnyParametersAccess) (*dto.CreateCollectionResponse, error)
	GetCollections(ctx context.Context, bunnyParametersAccess dto.BunnyParametersAccess) (*[]dto.BunnyCollection, error)
	UploadVideo(ctx context.Context, uploadVideoRequest dto.UploadVideoRequest, bunnyParametersAccess dto.BunnyParametersAccess) error
	CreateVideo(ctx context.Context, video dto.CreateVideoRequest, bunnyParametersAccess dto.BunnyParametersAccess) (*dto.CreateVideoResponse, error)
}
