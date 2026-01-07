package jobs

import (
	"context"
	"sync"
	"time"

	"github.com/memberclass-backend-golang/internal/domain/ports"
	"github.com/robfig/cron/v3"
)

type ScheduledJob struct {
	Job      ports.Job
	Schedule string
}

type Scheduler struct {
	logger   ports.Logger
	cron     *cron.Cron
	jobs     []ScheduledJob
	mu       sync.Mutex
	running  bool
}

func NewScheduler(logger ports.Logger) *Scheduler {
	return &Scheduler{
		logger: logger,
		cron:   cron.New(cron.WithSeconds()),
		jobs:   make([]ScheduledJob, 0),
	}
}

func (s *Scheduler) AddJob(job ports.Job, schedule string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.cron.AddFunc(schedule, func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()

		s.logger.Info("Executando job: " + job.Name())

		if err := job.Execute(ctx); err != nil {
			s.logger.Error("Falha na execução do job: " + job.Name() + " - " + err.Error())
		} else {
			s.logger.Info("Job executado com sucesso: " + job.Name())
		}
	})

	if err != nil {
		return err
	}

	s.jobs = append(s.jobs, ScheduledJob{
		Job:      job,
		Schedule: schedule,
	})

	s.logger.Info("Job agendado: " + job.Name() + " com schedule: " + schedule)
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

	s.logger.Info("Scheduler iniciado")
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

	s.logger.Info("Parando scheduler...")
	ctx := s.cron.Stop()
	<-ctx.Done()
	s.logger.Info("Scheduler parado")
}

