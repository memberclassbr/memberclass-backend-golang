package transcription

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/memberclass-backend-golang/internal/domain/dto"
	"github.com/memberclass-backend-golang/internal/domain/dto/request"
	"github.com/memberclass-backend-golang/internal/domain/ports"
)

type TranscriptionStatusCheckerJob struct {
	aiLessonUseCase ports.AILessonUseCase
	cache           ports.Cache
	logger          ports.Logger
	httpClient      *http.Client
	apiURL          string
}

func NewTranscriptionStatusCheckerJob(
	aiLessonUseCase ports.AILessonUseCase,
	cache ports.Cache,
	logger ports.Logger,
) *TranscriptionStatusCheckerJob {
	apiURL := os.Getenv("TRANSCRIPTION_API_URL")
	if apiURL == "" {
		logger.Error("TRANSCRIPTION_API_URL not configured")
	}

	return &TranscriptionStatusCheckerJob{
		aiLessonUseCase: aiLessonUseCase,
		cache:           cache,
		logger:          logger,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		apiURL: apiURL,
	}
}

func (j *TranscriptionStatusCheckerJob) Name() string {
	return "transcription-status-checker-job"
}

func (j *TranscriptionStatusCheckerJob) Execute(ctx context.Context) error {
	if j.apiURL == "" {
		j.logger.Error("TRANSCRIPTION_API_URL not configured, skipping job execution")
		return nil
	}

	jobListKey := "transcription:jobs:list"
	jobListData, err := j.cache.Get(ctx, jobListKey)
	if err != nil {
		if strings.Contains(err.Error(), "nil") {
			j.logger.Info("No pending jobs to check")
			return nil
		}
		j.logger.Error("Error fetching job list: " + err.Error())
		return err
	}

	if jobListData == "" {
		j.logger.Info("No pending jobs to check")
		return nil
	}

	var jobIDs []string
	if err := json.Unmarshal([]byte(jobListData), &jobIDs); err != nil {
		j.logger.Error("Error decoding job list: " + err.Error())
		return err
	}

	j.logger.Info(fmt.Sprintf("Checking status of %d jobs", len(jobIDs)))

	var activeJobIDs []string
	for _, jobID := range jobIDs {
		if err := j.checkJobStatus(ctx, jobID); err != nil {
			j.logger.Error(fmt.Sprintf("Error checking job %s: %s", jobID, err.Error()))
			activeJobIDs = append(activeJobIDs, jobID)
			continue
		}

		key := fmt.Sprintf("transcription:job:%s", jobID)
		exists, err := j.cache.Exists(ctx, key)
		if err == nil && exists {
			activeJobIDs = append(activeJobIDs, jobID)
		}
	}

	if len(activeJobIDs) != len(jobIDs) {
		if len(activeJobIDs) == 0 {
			if err := j.cache.Delete(ctx, jobListKey); err != nil {
				j.logger.Error("Error deleting empty job list: " + err.Error())
			}
		} else {
			updatedListData, err := json.Marshal(activeJobIDs)
			if err != nil {
				j.logger.Error("Error serializing updated job list: " + err.Error())
			} else {
				ttl := 24 * time.Hour
				if err := j.cache.Set(ctx, jobListKey, string(updatedListData), ttl); err != nil {
					j.logger.Error("Error updating job list: " + err.Error())
				}
			}
		}
	}

	return nil
}

func (j *TranscriptionStatusCheckerJob) checkJobStatus(ctx context.Context, jobID string) error {
	key := fmt.Sprintf("transcription:job:%s", jobID)
	jobDataStr, err := j.cache.Get(ctx, key)
	if err != nil {
		if strings.Contains(err.Error(), "nil") {
			return nil
		}
		return fmt.Errorf("error fetching job data: %w", err)
	}

	var jobData dto.TranscriptionJobData
	if err := json.Unmarshal([]byte(jobDataStr), &jobData); err != nil {
		return fmt.Errorf("error decoding job data: %w", err)
	}

	statusResponse, err := j.getJobStatusFromAPI(ctx, jobID)
	if err != nil {
		return fmt.Errorf("error fetching status from API: %w", err)
	}

	allCompleted := true
	for _, lessonStatus := range statusResponse.Lessons {
		if lessonStatus.Status == "COMPLETED" {
			updateReq := request.UpdateLessonTranscriptionRequest{
				TranscriptionCompleted: true,
			}
			if _, err := j.aiLessonUseCase.UpdateTranscriptionStatus(ctx, lessonStatus.LessonID, updateReq); err != nil {
				j.logger.Error(fmt.Sprintf("Error updating lesson %s: %s", lessonStatus.LessonID, err.Error()))
				continue
			}
			j.logger.Info(fmt.Sprintf("Lesson %s marked as transcriptionCompleted=true", lessonStatus.LessonID))
		} else if lessonStatus.Status == "FAILED" {
			errorMsg := "unknown error"
			if lessonStatus.ErrorMessage != nil {
				errorMsg = *lessonStatus.ErrorMessage
			}
			j.logger.Error(fmt.Sprintf("Lesson %s failed transcription: %s", lessonStatus.LessonID, errorMsg))
		} else {
			allCompleted = false
		}
	}

	if allCompleted {
		if err := j.cache.Delete(ctx, key); err != nil {
			j.logger.Error(fmt.Sprintf("Error deleting job %s from Redis: %s", jobID, err.Error()))
		} else {
			j.logger.Info(fmt.Sprintf("Job %s removed from Redis (all lessons completed)", jobID))
		}
	}

	return nil
}

func (j *TranscriptionStatusCheckerJob) getJobStatusFromAPI(ctx context.Context, jobID string) (*dto.TranscriptionJobStatusResponse, error) {
	url := fmt.Sprintf("%s/api/jobs/%s/status", j.apiURL, jobID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	resp, err := j.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error making request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error: status %d, body: %s", resp.StatusCode, string(body))
	}

	var statusResponse dto.TranscriptionJobStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&statusResponse); err != nil {
		return nil, fmt.Errorf("error decoding response: %w", err)
	}

	return &statusResponse, nil
}

