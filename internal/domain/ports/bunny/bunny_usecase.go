package bunny

import (
	"context"

	"github.com/memberclass-backend-golang/internal/domain/dto"
	"github.com/memberclass-backend-golang/internal/domain/dto/response"
)

type UploadVideoBunnyCdnUseCase interface {
	Execute(ctx context.Context, bunnyParams dto.BunnyParametersAccess, fileBytes []byte, title string) (*response.UploadVideoResponse, error)
}
