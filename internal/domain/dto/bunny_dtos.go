package dto

type BunnyCollection struct {
	VideoLibraryID     int    `json:"videoLibraryId"`
	GUID               string `json:"guid"`
	Name               string `json:"name"`
	VideoCount         int    `json:"videoCount"`
	TotalSize          int64  `json:"totalSize"`
	PreviewVideoIds    string `json:"previewVideoIds"`
	PreviewImageUrls   []string `json:"previewImageUrls"`
}

type BunnyCollectionsResponse struct {
	TotalItems    int                `json:"totalItems"`
	CurrentPage   int                `json:"currentPage"`
	ItemsPerPage  int                `json:"itemsPerPage"`
	Items         []BunnyCollection  `json:"items"`
}


type CreateVideoRequest struct {
    Title        string `json:"Title"`
    CollectionID string `json:"collectionId,omitempty"`
}


type CreateVideoResponse struct {
    GUID string `json:"guid"`
}


type UploadVideoRequest struct {
	GUID string `json:"guid"`
	File []byte `json:"file"`
	ContentType string `json:"contentType"`
}


type BunnyParametersAccess struct {
	LibraryID string `json:"libraryId"`
	LibraryApiKey string `json:"libraryApiKey"`
}

type CreateCollectionRequest struct {
    Name string `json:"Name"`
}

type CreateCollectionResponse struct {
    GUID string `json:"guid"`
}