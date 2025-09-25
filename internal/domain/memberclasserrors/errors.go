package memberclasserrors

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
	ErrTenantNotFound = NewMemberClassError(404, "tenant not found")
	ErrTenantIDEmpty  = NewMemberClassError(400, "tenant ID is empty")
)
