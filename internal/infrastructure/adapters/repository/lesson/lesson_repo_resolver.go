package lesson

import (
	"context"
	"fmt"

	"github.com/memberclass-backend-golang/internal/domain/memberclasserrors"
	"github.com/memberclass-backend-golang/internal/domain/ports"
	lessonports "github.com/memberclass-backend-golang/internal/domain/ports/lesson"
	"github.com/memberclass-backend-golang/internal/infrastructure/adapters/database"
)

type lessonRepoResolver struct {
	repos      map[string]lessonports.LessonRepository
	defaultKey string
	logger     ports.Logger
}

// NewLessonRepoResolver creates a LessonRepoResolver from a DBMap.
// Each bucket gets its own LessonRepository backed by the corresponding DB connection.
func NewLessonRepoResolver(dbMap database.DBMap, logger ports.Logger) lessonports.LessonRepoResolver {
	repos := make(map[string]lessonports.LessonRepository, len(dbMap))

	for bucket, db := range dbMap {
		repos[bucket] = NewLessonRepository(db, logger)
		logger.Info(fmt.Sprintf("LessonRepository registered for bucket '%s'", bucket))
	}

	// Default is memberclass
	defaultKey := "memberclass"
	if _, ok := repos[defaultKey]; !ok {
		// Fallback to first available
		for k := range repos {
			defaultKey = k
			break
		}
	}

	return &lessonRepoResolver{
		repos:      repos,
		defaultKey: defaultKey,
		logger:     logger,
	}
}

func (r *lessonRepoResolver) Resolve(bucket string) lessonports.LessonRepository {
	if repo, ok := r.repos[bucket]; ok {
		return repo
	}
	r.logger.Warn(fmt.Sprintf("No repository found for bucket '%s', using default '%s'", bucket, r.defaultKey))
	return r.repos[r.defaultKey]
}

func (r *lessonRepoResolver) All() map[string]lessonports.LessonRepository {
	copy := make(map[string]lessonports.LessonRepository, len(r.repos))
	for k, v := range r.repos {
		copy[k] = v
	}
	return copy
}

func (r *lessonRepoResolver) Default() lessonports.LessonRepository {
	return r.repos[r.defaultKey]
}

func (r *lessonRepoResolver) FindByLessonID(ctx context.Context, lessonID string) (lessonports.LessonRepository, string, error) {
	// Try default first (most common case)
	defaultRepo := r.repos[r.defaultKey]
	lesson, err := defaultRepo.GetByID(ctx, lessonID)
	if err == nil && lesson != nil {
		return defaultRepo, r.defaultKey, nil
	}

	// Try other repos
	for bucket, repo := range r.repos {
		if bucket == r.defaultKey {
			continue
		}
		lesson, err := repo.GetByID(ctx, lessonID)
		if err == nil && lesson != nil {
			r.logger.Info(fmt.Sprintf("Lesson '%s' found in bucket '%s'", lessonID, bucket))
			return repo, bucket, nil
		}
	}

	return nil, "", &memberclasserrors.MemberClassError{
		Code:    404,
		Message: "lesson not found in any database",
	}
}
