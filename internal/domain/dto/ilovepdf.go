package dto

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
