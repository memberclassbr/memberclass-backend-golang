package transcription

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
)

// RunCronEnqueue is the daily-cron entry point. For every AI-enabled
// tenant, it lists lessons with transcriptionCompleted = false and a
// Bunny mediaUrl, then enqueues one TRANSCRIPTION job per lesson on the
// Railway pgvector jobs table. Idempotency relies on the fact that an
// already-COMPLETED video skips work inside executeJob via the
// UNIQUE (tenant_id, source_url) index — re-enqueueing is safe but
// wasteful, so we still skip lessons with the flag flipped.
//
// Wire this as a *jobs.Job in cmd/api/main.go so the existing scheduler
// drives it at cronSchedule (22:00 daily).
func (f *Feature) RunCronEnqueue(ctx context.Context) error {
	if err := f.preflight(); err != nil {
		f.log.Warn("transcription.cron.preflight_skipped", "error", err.Error())
		return nil
	}

	tenants, err := f.listAITenants(ctx)
	if err != nil {
		return fmt.Errorf("list ai tenants: %w", err)
	}
	if len(tenants) == 0 {
		f.log.Info("transcription.cron.no_ai_tenants")
		return nil
	}

	totalEnqueued := 0
	for _, t := range tenants {
		n, err := f.enqueueForTenant(ctx, t)
		if err != nil {
			f.log.Error("transcription.cron.tenant_failed",
				"tenant", t.ID, "error", err.Error())
			continue
		}
		if n > 0 {
			f.log.Info("transcription.cron.tenant_enqueued", "tenant", t.ID, "jobs", n)
		}
		totalEnqueued += n
	}
	f.log.Info("transcription.cron.completed",
		"tenants", len(tenants), "jobs", totalEnqueued)
	return nil
}

type aiTenant struct {
	ID      string
	Name    string
	LibID   string
	LibKey  string
}

func (f *Feature) listAITenants(ctx context.Context) ([]aiTenant, error) {
	rows, err := f.memberclassDB.QueryContext(ctx, sqlSelectAITenants)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []aiTenant
	for rows.Next() {
		var (
			t      aiTenant
			libID  *string
			libKey *string
		)
		if err := rows.Scan(&t.ID, &t.Name, &libID, &libKey); err != nil {
			return nil, err
		}
		if libID != nil {
			t.LibID = *libID
		}
		if libKey != nil {
			t.LibKey = *libKey
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

type unprocessedLesson struct {
	ID         string
	Name       string
	MediaURL   string
	CourseID   string
	CourseName string
}

func (f *Feature) enqueueForTenant(ctx context.Context, t aiTenant) (int, error) {
	if t.LibID == "" || t.LibKey == "" {
		f.log.Warn("transcription.cron.tenant_missing_bunny", "tenant", t.ID)
		return 0, nil
	}
	rows, err := f.memberclassDB.QueryContext(ctx, sqlSelectUnprocessedLessons, t.ID)
	if err != nil {
		return 0, fmt.Errorf("select lessons: %w", err)
	}
	defer rows.Close()

	var lessons []unprocessedLesson
	for rows.Next() {
		var (
			l                                                         unprocessedLesson
			lessonType, thumbnail, content                            *string
			moduleID, moduleName, sectionID, sectionName              string
			vitrineID, vitrineName                                    string
		)
		if err := rows.Scan(
			&l.ID, &l.Name, new(string),
			&lessonType, &l.MediaURL, &thumbnail, &content,
			&moduleID, &moduleName, &sectionID, &sectionName,
			&l.CourseID, &l.CourseName, &vitrineID, &vitrineName,
		); err != nil {
			return 0, fmt.Errorf("scan lesson: %w", err)
		}
		lessons = append(lessons, l)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if len(lessons) == 0 {
		return 0, nil
	}

	enqueued := 0
	for _, l := range lessons {
		payload, err := json.Marshal(jobPayload{
			LessonID: l.ID,
			TenantID: t.ID,
			VideoURL: l.MediaURL,
			CourseID: l.CourseID,
			Title:    l.Name,
		})
		if err != nil {
			return enqueued, fmt.Errorf("marshal payload for lesson %s: %w", l.ID, err)
		}
		if _, err := f.transcriptionDB.ExecContext(ctx, sqlInsertJob,
			uuid.NewString(), t.ID, 0, payload, 3,
		); err != nil {
			f.log.Error("transcription.cron.enqueue_failed",
				"tenant", t.ID, "lesson", l.ID, "error", err.Error())
			continue
		}
		enqueued++
	}
	return enqueued, nil
}

// scheduledJob adapts RunCronEnqueue to the ports.Job interface that the
// existing scheduler in internal/application/jobs expects. Wire one of
// these in cmd/api/main.go to replace the legacy transcription.TranscriptionJob.
type scheduledJob struct {
	feat *Feature
}

func NewScheduledJob(f *Feature) *scheduledJob { return &scheduledJob{feat: f} }

func (s *scheduledJob) Name() string { return "transcription-enqueue" }

func (s *scheduledJob) Execute(ctx context.Context) error {
	return s.feat.RunCronEnqueue(ctx)
}
