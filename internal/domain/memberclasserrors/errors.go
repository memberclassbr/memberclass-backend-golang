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

	//Errors Lessons
	ErrLessonNotFound   = errors.New("lesson not found")
	ErrPDFAssetNotFound = errors.New("PDF asset not found")
	ErrPDFPageNotFound  = errors.New("PDF page not found")
)
