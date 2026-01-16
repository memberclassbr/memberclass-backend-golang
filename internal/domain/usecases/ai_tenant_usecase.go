package usecases

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/memberclass-backend-golang/internal/domain/dto"
	"github.com/memberclass-backend-golang/internal/domain/dto/request"
	"github.com/memberclass-backend-golang/internal/domain/dto/response"
	"github.com/memberclass-backend-golang/internal/domain/memberclasserrors"
	"github.com/memberclass-backend-golang/internal/domain/ports"
)

type AITenantUseCaseImpl struct {
	tenantRepository ports.TenantRepository
	aiLessonUseCase  ports.AILessonUseCase
	logger           ports.Logger
	httpClient       *http.Client
	apiURL           string
}

func NewAITenantUseCase(
	tenantRepository ports.TenantRepository,
	aiLessonUseCase ports.AILessonUseCase,
	logger ports.Logger,
) ports.AITenantUseCase {
	apiURL := os.Getenv("TRANSCRIPTION_API_URL")
	if apiURL == "" {
		logger.Warn("TRANSCRIPTION_API_URL not configured")
	}

	return &AITenantUseCaseImpl{
		tenantRepository: tenantRepository,
		aiLessonUseCase:  aiLessonUseCase,
		logger:           logger,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		apiURL: apiURL,
	}
}

func (uc *AITenantUseCaseImpl) GetTenantsWithAIEnabled(ctx context.Context) (*response.AITenantsResponse, error) {
	tenants, err := uc.tenantRepository.FindAllWithAIEnabled(ctx)
	if err != nil {
		return nil, err
	}

	tenantData := make([]response.AITenantData, len(tenants))
	for i, tenant := range tenants {
		var bunnyLibraryID *string
		var bunnyLibraryApiKey *string

		if tenant.BunnyLibraryID != nil {
			bunnyLibraryID = tenant.BunnyLibraryID
		}
		if tenant.BunnyLibraryApiKey != nil {
			bunnyLibraryApiKey = tenant.BunnyLibraryApiKey
		}

		tenantData[i] = response.AITenantData{
			ID:                 tenant.ID,
			Name:               tenant.Name,
			AIEnabled:          tenant.AIEnabled,
			BunnyLibraryID:     bunnyLibraryID,
			BunnyLibraryApiKey: bunnyLibraryApiKey,
		}
	}

	return &response.AITenantsResponse{
		Tenants: tenantData,
		Total:   len(tenantData),
	}, nil
}

func (uc *AITenantUseCaseImpl) ProcessLessonsTenant(ctx context.Context, req request.ProcessLessonsTenantRequest) (*response.ProcessLessonsTenantResponse, error) {
	if req.TenantID == "" {
		return nil, &memberclasserrors.MemberClassError{
			Code:    400,
			Message: "tenantId é obrigatório",
		}
	}

	if uc.apiURL == "" {
		return nil, &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "TRANSCRIPTION_API_URL não está configurada",
		}
	}

	tenant, err := uc.tenantRepository.FindByID(req.TenantID)
	if err != nil {
		var memberClassErr *memberclasserrors.MemberClassError
		if errors.As(err, &memberClassErr) && memberClassErr.Code == 404 {
			return nil, &memberclasserrors.MemberClassError{
				Code:    404,
				Message: "Tenant não encontrado",
			}
		}
		return nil, err
	}

	if !tenant.AIEnabled {
		return nil, &memberclasserrors.MemberClassError{
			Code:    403,
			Message: "IA não está habilitada para este tenant",
		}
	}

	lessonsReq := request.GetAILessonsRequest{
		TenantID:        req.TenantID,
		OnlyUnprocessed: true,
	}

	lessonsResponse, err := uc.aiLessonUseCase.GetLessons(ctx, lessonsReq)
	if err != nil {
		return nil, err
	}

	if lessonsResponse.Total == 0 {
		return &response.ProcessLessonsTenantResponse{
			Success:      false,
			Message:      "Nenhuma lesson não processada encontrada para este tenant",
			TenantID:     req.TenantID,
			LessonsCount: 0,
		}, nil
	}

	payload := uc.buildPayload(lessonsResponse.Lessons, req.TenantID)

	jobResponse, err := uc.sendToAPI(ctx, payload)
	if err != nil {
		return nil, &memberclasserrors.MemberClassError{
			Code:    500,
			Message: fmt.Sprintf("Erro ao enviar para API de transcrição: %s", err.Error()),
		}
	}

	return &response.ProcessLessonsTenantResponse{
		Success:      true,
		Message:      "Job de transcrição criado com sucesso",
		JobID:        &jobResponse.JobID,
		LessonsCount: len(lessonsResponse.Lessons),
		TenantID:     req.TenantID,
	}, nil
}

func (uc *AITenantUseCaseImpl) buildPayload(lessons []response.AILessonData, tenantID string) dto.TranscriptionJobRequest {
	lessonsData := make([]dto.TranscriptionLessonData, len(lessons))
	for i, lesson := range lessons {
		lessonsData[i] = dto.TranscriptionLessonData{
			ID:                     lesson.ID,
			Name:                   lesson.Name,
			Slug:                   lesson.Slug,
			Type:                   lesson.Type,
			MediaURL:               lesson.MediaURL,
			Thumbnail:              lesson.Thumbnail,
			Content:                lesson.Content,
			TranscriptionCompleted: lesson.TranscriptionCompleted,
			ModuleID:               lesson.ModuleID,
			ModuleName:             lesson.ModuleName,
			SectionID:              lesson.SectionID,
			SectionName:            lesson.SectionName,
			CourseID:               lesson.CourseID,
			CourseName:             lesson.CourseName,
			VitrineID:              lesson.VitrineID,
			VitrineName:            lesson.VitrineName,
		}
	}
	return dto.TranscriptionJobRequest{
		Lessons:  lessonsData,
		TenantID: tenantID,
	}
}

func (uc *AITenantUseCaseImpl) sendToAPI(ctx context.Context, payload dto.TranscriptionJobRequest) (*dto.TranscriptionJobResponse, error) {
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("erro ao serializar payload: %w", err)
	}

	url := fmt.Sprintf("%s/api/v2/extract-and-embed", uc.apiURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("erro ao criar request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := uc.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("erro ao fazer request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("erro na API: status %d, body: %s", resp.StatusCode, string(body))
	}

	var jobResponse dto.TranscriptionJobResponse
	if err := json.NewDecoder(resp.Body).Decode(&jobResponse); err != nil {
		return nil, fmt.Errorf("erro ao decodificar resposta: %w", err)
	}

	return &jobResponse, nil
}
