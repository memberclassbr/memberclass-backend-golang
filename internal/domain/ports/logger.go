package ports

import "github.com/memberclass-backend-golang/internal/platform/logger"

// Logger aliases the platform logger contract.
//
// The interface now lives next to its implementation in internal/platform.
// This alias keeps the not-yet-migrated packages under internal/domain and
// internal/application compiling against ports.Logger; it disappears with them
// in the cleanup that ends the vertical-slice migration. New code should import
// internal/platform/logger directly.
type Logger = logger.Logger
