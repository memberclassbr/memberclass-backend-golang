package request

type UpdateCommentRequest struct {
	Answer    string `json:"answer"`
	Published *bool  `json:"published"`
}
