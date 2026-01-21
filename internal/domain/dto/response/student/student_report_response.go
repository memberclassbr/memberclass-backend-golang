package student

type LessonWatched struct {
	AulaID        string `json:"aula_id"`
	Titulo        string `json:"titulo"`
	DataAssistida string `json:"data_assistida"`
}

type StudentReport struct {
	AlunoIDMemberClass        string          `json:"aluno_id_member_class"`
	Email                     string          `json:"email"`
	Cpf                       string          `json:"cpf"`
	DataCadastro              string          `json:"data_cadastro"`
	EntregasVinculadas        []string        `json:"entregas_vinculadas"`
	UltimoAcesso              *string         `json:"ultimo_acesso"`
	QuantidadeAulasAssistidas int             `json:"quantidade_aulas_assistidas"`
	AulasAssistidas           []LessonWatched `json:"aulas_assistidas"`
}

type StudentReportPagination struct {
	Page        int  `json:"page"`
	Limit       int  `json:"limit"`
	TotalCount  int  `json:"totalCount"`
	TotalPages  int  `json:"totalPages"`
	HasNextPage bool `json:"hasNextPage"`
	HasPrevPage bool `json:"hasPrevPage"`
}

type StudentReportResponse struct {
	Alunos     []StudentReport         `json:"alunos"`
	Pagination StudentReportPagination `json:"pagination"`
}
