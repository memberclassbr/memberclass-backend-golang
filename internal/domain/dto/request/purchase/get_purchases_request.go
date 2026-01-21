package purchase

import "errors"

type GetUserPurchasesRequest struct {
	Email string
	Page  int
	Limit int
	Type  string
}

func (r *GetUserPurchasesRequest) Validate() error {
	if r.Email == "" {
		return errors.New("email é obrigatório")
	}
	if r.Page < 1 {
		return errors.New("page deve ser >= 1")
	}
	if r.Limit < 1 || r.Limit > 100 {
		return errors.New("limit deve ser entre 1 e 100")
	}
	return nil
}
