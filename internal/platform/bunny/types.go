// Package bunny is the HTTP client for the Bunny Stream API: creating videos,
// uploading their bytes, and managing collections. Credentials are per tenant
// and travel in each call rather than being held by the client.
package bunny

import "context"

type Collection struct {
	VideoLibraryID   int      `json:"videoLibraryId"`
	GUID             string   `json:"guid"`
	Name             string   `json:"name"`
	VideoCount       int      `json:"videoCount"`
	TotalSize        int64    `json:"totalSize"`
	PreviewVideoIds  string   `json:"previewVideoIds"`
	PreviewImageUrls []string `json:"previewImageUrls"`
}

type CollectionsResponse struct {
	TotalItems   int          `json:"totalItems"`
	CurrentPage  int          `json:"currentPage"`
	ItemsPerPage int          `json:"itemsPerPage"`
	Items        []Collection `json:"items"`
}

type CreateVideoRequest struct {
	Title        string `json:"Title"`
	CollectionID string `json:"collectionId,omitempty"`
}

type CreateVideoResponse struct {
	GUID string `json:"guid"`
}

type UploadVideoRequest struct {
	GUID        string `json:"guid"`
	File        []byte `json:"file"`
	ContentType string `json:"contentType"`
}

type ParametersAccess struct {
	LibraryID     string `json:"libraryId"`
	LibraryApiKey string `json:"libraryApiKey"`
}

type CreateCollectionRequest struct {
	Name string `json:"Name"`
}

type CreateCollectionResponse struct {
	GUID string `json:"guid"`
}

// Service is the contract satisfied by the client in this package. It lives
// next to the implementation: there is one client, and it carries no business
// rule worth a separate ports package.
type Service interface {
	CreateCollection(ctx context.Context, req CreateCollectionRequest, access ParametersAccess) (*CreateCollectionResponse, error)
	GetCollections(ctx context.Context, access ParametersAccess) (*CollectionsResponse, error)
	UploadVideo(ctx context.Context, req UploadVideoRequest, access ParametersAccess) error
	CreateVideo(ctx context.Context, video CreateVideoRequest, access ParametersAccess) (*CreateVideoResponse, error)
}
