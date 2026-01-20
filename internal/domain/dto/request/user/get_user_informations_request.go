package user

import "errors"

type GetUserInformationsRequest struct {
	Email string
	Page  int
	Limit int
}

func (r *GetUserInformationsRequest) Validate() error {
	if r.Page < 1 {
		return errors.New("page deve ser >= 1")
	}
	if r.Limit < 1 || r.Limit > 100 {
		return errors.New("limit deve ser entre 1 e 100")
	}
	return nil
}
