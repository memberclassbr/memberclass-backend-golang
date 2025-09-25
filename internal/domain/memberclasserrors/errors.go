package memberclasserrors

type MemberClassError struct {
	Code    int
	Message string
	Err     error
}

func (e *MemberClassError) Error() string {
	return e.Message
}

func NewMemberClassError(code int, message string, err error) error {
	return &MemberClassError{
		Code:    code,
		Message: message,
	}
}
