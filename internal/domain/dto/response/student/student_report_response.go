package student

import "github.com/memberclass-backend-golang/internal/domain/dto"

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

type StudentReportResponse struct {
	Alunos     []StudentReport    `json:"alunos"`
	Pagination dto.PaginationMeta `json:"pagination"`
}
