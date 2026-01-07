package request

type GetAILessonsRequest struct {
	TenantID        string
	OnlyUnprocessed bool
}

func ParseGetAILessonsRequest(tenantID, onlyUnprocessed string) *GetAILessonsRequest {
	onlyUnprocessedBool := onlyUnprocessed == "true"
	return &GetAILessonsRequest{
		TenantID:        tenantID,
		OnlyUnprocessed: onlyUnprocessedBool,
	}
}

