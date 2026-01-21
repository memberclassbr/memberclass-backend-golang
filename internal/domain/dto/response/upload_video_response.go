package response

type UploadVideoResponse struct {
	OK       bool   `json:"ok"`
	MediaURL string `json:"mediaUrl"`
	GUID     string `json:"guid"`
	Title    string `json:"title"`
}
