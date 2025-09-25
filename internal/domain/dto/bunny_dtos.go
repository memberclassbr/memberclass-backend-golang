package dto

type BunnyCollection struct {
	GUID string `json:"guid"`
    Name string `json:"name"`
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