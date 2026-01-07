package transcription

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/memberclass-backend-golang/internal/domain/dto"
	"github.com/memberclass-backend-golang/internal/domain/dto/request"
	"github.com/memberclass-backend-golang/internal/domain/dto/response"
	"github.com/memberclass-backend-golang/internal/domain/ports"
)

type TranscriptionJob struct {
	aiTenantUseCase ports.AITenantUseCase
	aiLessonUseCase ports.AILessonUseCase
	cache           ports.Cache
	logger          ports.Logger
	httpClient      *http.Client
	apiURL          string
}

func NewTranscriptionJob(
	aiTenantUseCase ports.AITenantUseCase,
	aiLessonUseCase ports.AILessonUseCase,
	cache ports.Cache,
	logger ports.Logger,
) *TranscriptionJob {
	apiURL := os.Getenv("TRANSCRIPTION_API_URL")
	if apiURL == "" {
		logger.Error("TRANSCRIPTION_API_URL not configured")
	}

	return &TranscriptionJob{
		aiTenantUseCase: aiTenantUseCase,
		aiLessonUseCase: aiLessonUseCase,
		cache:           cache,
		logger:          logger,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		apiURL: apiURL,
	}
}

func (j *TranscriptionJob) Name() string {
	return "transcription-job"
}

func (j *TranscriptionJob) Execute(ctx context.Context) error {
	if j.apiURL == "" {
		j.logger.Error("TRANSCRIPTION_API_URL not configured, skipping job execution")
		return nil
	}

	tenants, err := j.aiTenantUseCase.GetTenantsWithAIEnabled(ctx)
	if err != nil {
		j.logger.Error("Error fetching tenants with AI enabled: " + err.Error())
		return err
	}

	j.logger.Info(fmt.Sprintf("Processing %d tenants with AI enabled", tenants.Total))

	for _, tenant := range tenants.Tenants {
		if err := j.processTenant(ctx, tenant); err != nil {
			j.logger.Error(fmt.Sprintf("Error processing tenant %s: %s", tenant.ID, err.Error()))
			continue
		}
	}

	return nil
}

func (j *TranscriptionJob) processTenant(ctx context.Context, tenant response.AITenantData) error {
	if j.hasPendingJobForTenant(ctx, tenant.ID) {
		j.logger.Info(fmt.Sprintf("Tenant %s already has a pending transcription job, skipping", tenant.ID))
		return nil
	}

	req := request.GetAILessonsRequest{
		TenantID:        tenant.ID,
		OnlyUnprocessed: true,
	}

	lessonsResponse, err := j.aiLessonUseCase.GetLessons(ctx, req)
	if err != nil {
		return fmt.Errorf("error fetching lessons: %w", err)
	}

	if lessonsResponse.Total == 0 {
		j.logger.Info(fmt.Sprintf("No unprocessed lessons for tenant %s", tenant.ID))
		return nil
	}

	j.logger.Info(fmt.Sprintf("Found %d lessons to process for tenant %s", len(lessonsResponse.Lessons), tenant.ID))

	payload := j.buildPayload(lessonsResponse.Lessons, tenant.ID)

	jobResponse, err := j.sendToAPI(ctx, payload)
	if err != nil {
		return fmt.Errorf("error sending to API: %w", err)
	}

	if err := j.saveJobToRedis(ctx, jobResponse.JobID, tenant.ID, lessonsResponse.Lessons); err != nil {
		return fmt.Errorf("error saving job to Redis: %w", err)
	}

	j.logger.Info(fmt.Sprintf("Job %s created successfully for tenant %s", jobResponse.JobID, tenant.ID))
	return nil
}

func (j *TranscriptionJob) hasPendingJobForTenant(ctx context.Context, tenantID string) bool {
	jobListKey := "transcription:jobs:list"
	jobListData, err := j.cache.Get(ctx, jobListKey)
	if err != nil {
		return false
	}

	if jobListData == "" {
		return false
	}

	var jobIDs []string
	if err := json.Unmarshal([]byte(jobListData), &jobIDs); err != nil {
		return false
	}

	for _, jobID := range jobIDs {
		key := fmt.Sprintf("transcription:job:%s", jobID)
		jobDataStr, err := j.cache.Get(ctx, key)
		if err != nil {
			continue
		}

		var jobData dto.TranscriptionJobData
		if err := json.Unmarshal([]byte(jobDataStr), &jobData); err != nil {
			continue
		}

		if jobData.TenantID == tenantID {
			return true
		}
	}

	return false
}

func (j *TranscriptionJob) buildPayload(lessons []response.AILessonData, tenantID string) dto.TranscriptionJobRequest {
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

func (j *TranscriptionJob) sendToAPI(ctx context.Context, payload dto.TranscriptionJobRequest) (*dto.TranscriptionJobResponse, error) {
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("error serializing payload: %w", err)
	}

	url := fmt.Sprintf("%s/api/v2/extract-and-embed", j.apiURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := j.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error making request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error: status %d, body: %s", resp.StatusCode, string(body))
	}

	var jobResponse dto.TranscriptionJobResponse
	if err := json.NewDecoder(resp.Body).Decode(&jobResponse); err != nil {
		return nil, fmt.Errorf("error decoding response: %w", err)
	}

	return &jobResponse, nil
}

func (j *TranscriptionJob) saveJobToRedis(ctx context.Context, jobID, tenantID string, lessons []response.AILessonData) error {
	lessonIDs := make([]string, len(lessons))
	for i, lesson := range lessons {
		lessonIDs[i] = lesson.ID
	}

	jobData := dto.TranscriptionJobData{
		JobID:     jobID,
		TenantID:  tenantID,
		LessonIDs: lessonIDs,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}

	jsonData, err := json.Marshal(jobData)
	if err != nil {
		return fmt.Errorf("error serializing job data: %w", err)
	}

	key := fmt.Sprintf("transcription:job:%s", jobID)
	ttl := 24 * time.Hour

	if err := j.cache.Set(ctx, key, string(jsonData), ttl); err != nil {
		return fmt.Errorf("error saving to Redis: %w", err)
	}

	jobListKey := "transcription:jobs:list"
	existingList, _ := j.cache.Get(ctx, jobListKey)
	var jobIDs []string
	if existingList != "" {
		if err := json.Unmarshal([]byte(existingList), &jobIDs); err != nil {
			j.logger.Error("Error decoding job list: " + err.Error())
			jobIDs = []string{}
		}
	}

	jobIDs = append(jobIDs, jobID)
	newListData, err := json.Marshal(jobIDs)
	if err != nil {
		return fmt.Errorf("error serializing job list: %w", err)
	}

	if err := j.cache.Set(ctx, jobListKey, string(newListData), ttl); err != nil {
		return fmt.Errorf("error updating job list: %w", err)
	}

	return nil
}

