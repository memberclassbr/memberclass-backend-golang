// Package logger provides the structured logger every other package writes to.
//
// The Logger interface lives next to its implementation rather than in a
// separate ports package: there is one logger, it has no business rules, and
// splitting the contract from the only thing that satisfies it bought nothing
// but an extra import.
package logger

// Logger is the logging contract used across the service.
type Logger interface {
	Debug(msg string, args ...any)
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
}
