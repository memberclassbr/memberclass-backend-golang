package lesson

import "context"

// LessonRepoResolver resolves the correct LessonRepository based on bucket name.
// Each bucket maps to a different database connection.
type LessonRepoResolver interface {
	// Resolve returns the LessonRepository for a given bucket name.
	// Falls back to Default() if bucket is unknown.
	Resolve(bucket string) LessonRepository

	// All returns all registered LessonRepository instances keyed by bucket name.
	All() map[string]LessonRepository

	// Default returns the default LessonRepository (memberclass / DB_DSN).
	Default() LessonRepository

	// FindByLessonID searches all repositories for a lesson by ID.
	// Returns the repository that contains the lesson and the bucket name.
	FindByLessonID(ctx context.Context, lessonID string) (LessonRepository, string, error)
}
