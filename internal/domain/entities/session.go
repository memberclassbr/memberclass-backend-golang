package entities

import "time"

type Session struct {
	ID           *string   `json:"id"`
	SessionToken *string   `json:"sessionToken"`
	UpdatedAt    time.Time `json:"updatedAt"`
	UserID       *string   `json:"userId"`
	TenantID     *string   `json:"tenantId"`
	Expires      time.Time `json:"expires"`
}
