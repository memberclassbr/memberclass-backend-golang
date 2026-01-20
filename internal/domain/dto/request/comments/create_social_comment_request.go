package comments

import "errors"

type CreateSocialCommentRequest struct {
	UserID     string  `json:"userId"`
	TopicID    string  `json:"topicId,omitempty"`
	PostID     string  `json:"postId,omitempty"`
	TenantID   string  `json:"tenantId,omitempty"`
	Title      string  `json:"title"`
	Content    string  `json:"content"`
	Image      *string `json:"image,omitempty"`
	VideoEmbed *string `json:"videoEmbed,omitempty"`
}

func (r *CreateSocialCommentRequest) Validate() error {
	if r.UserID == "" {
		return errors.New("userId é obrigatório")
	}
	if r.PostID == "" && r.TopicID == "" {
		return errors.New("topicId é obrigatório para criar post")
	}
	if r.Title == "" {
		return errors.New("title é obrigatório")
	}
	if r.Content == "" {
		return errors.New("content é obrigatório")
	}
	return nil
}
