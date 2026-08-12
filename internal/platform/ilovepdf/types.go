// Package ilovepdf is the client for the iLovePDF API, used to rasterise a
// lesson's PDF into page images.
//
// The API is task-based: authenticate, start a task, add the file, process,
// then download a zip of the rendered pages.
package ilovepdf

type AuthResponse struct {
	Token string `json:"token"`
}

type TaskResponse struct {
	Task   string `json:"task"`
	Server string `json:"server"`
}

type UploadResponse struct {
	ServerFilename string `json:"server_filename"`
}

type ProcessResponse struct {
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

type DownloadResponse struct {
	DownloadURL string `json:"download_url"`
}

// Service is the contract satisfied by the client in this package.
type Service interface {
	GetToken() (string, error)
	CreateTask(token string) (*TaskResponse, error)
	GetTokenAndCreateTask() (string, *TaskResponse, error)
	AddFile(token, taskID, pdfURL, server string) (string, error)
	ProcessTask(token, taskID, serverFilename, server string) error
	DownloadTask(token, taskID, server string) ([]byte, error)
	ExtractImagesFromZip(zipData []byte) ([]string, error)
}

type ApiKeyInfo struct {
	Key      string `json:"key"`
	LastUsed string `json:"last_used"`
}
