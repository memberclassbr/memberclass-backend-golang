package memberclasserrors

import "errors"

type MemberClassError struct {
	Code    int
	Message string
	Err     error
}

func (e *MemberClassError) Error() string {
	return e.Message
}

func NewMemberClassError(code int, message string) error {
	return &MemberClassError{
		Code:    code,
		Message: message,
	}
}

var (
	//Errors Tenant
	ErrTenantNotFound = NewMemberClassError(404, "tenant not found")
	ErrTenantIDEmpty  = NewMemberClassError(400, "tenant ID is empty")

	//Errors User
	ErrUserNotFound        = NewMemberClassError(404, "user not found")
	ErrUserNotInTenant     = NewMemberClassError(403, "user does not belong to this tenant")
	ErrUnauthorized        = NewMemberClassError(401, "unauthorized")
	ErrSessionTokenMissing = NewMemberClassError(401, "session token not found")
	ErrSessionTokenInvalid = NewMemberClassError(401, "invalid session token")
	ErrSessionExpired      = NewMemberClassError(401, "session expired")

	//Errors Lessons
	ErrLessonNotFound   = errors.New("lesson not found")
	ErrPDFAssetNotFound = errors.New("PDF asset not found")
	ErrPDFPageNotFound  = errors.New("PDF page not found")

	//Errors Comment
	ErrCommentNotFound = errors.New("comment not found")
)
