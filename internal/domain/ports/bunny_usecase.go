package ports

import (
	"context"

	"github.com/memberclass-backend-golang/internal/domain/dto"
)

type UploadVideoBunnyCdnUseCase interface {
	Execute(ctx context.Context, bunnyParams dto.BunnyParametersAccess, fileBytes []byte, title string) (*dto.UploadVideoResponse, error)
}
