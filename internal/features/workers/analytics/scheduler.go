package analytics

import (
	"context"
	"sync"
	"time"

	"github.com/memberclass-backend-golang/internal/platform/logger"
	"github.com/robfig/cron/v3"
)

// Job is anything the scheduler can run on a cron expression. It lives here
// because the analytics rollups are the only cron work in the service.
type Job interface {
	Execute(ctx context.Context) error
	Name() string
}

type ScheduledJob struct {
	Job      Job
	Schedule string
}

type Scheduler struct {
	logger  logger.Logger
	cron    *cron.Cron
	jobs    []ScheduledJob
	mu      sync.Mutex
	running bool
}

func NewScheduler(logger logger.Logger) *Scheduler {
	return &Scheduler{
		logger: logger,
		cron:   cron.New(cron.WithSeconds()),
		jobs:   make([]ScheduledJob, 0),
	}
}

func (s *Scheduler) AddJob(job Job, schedule string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.cron.AddFunc(schedule, func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()

		s.logger.Info("Executing job: " + job.Name())

		if err := job.Execute(ctx); err != nil {
			s.logger.Error("Job execution failed: " + job.Name() + " - " + err.Error())
		} else {
			s.logger.Info("Job executed successfully: " + job.Name())
		}
	})

	if err != nil {
		return err
	}

	s.jobs = append(s.jobs, ScheduledJob{
		Job:      job,
		Schedule: schedule,
	})

	s.logger.Info("Job scheduled: " + job.Name() + " with schedule: " + schedule)
	return nil
}

func (s *Scheduler) Start() {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.mu.Unlock()

	s.logger.Info("Scheduler started")
	s.cron.Start()
}

func (s *Scheduler) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false
	s.mu.Unlock()

	s.logger.Info("Stopping scheduler...")
	ctx := s.cron.Stop()
	<-ctx.Done()
	s.logger.Info("Scheduler stopped")
}
