package entities

import "time"

type User struct {
	ID                   string     `json:"id"`
	Name                 *string    `json:"name"`
	Username             *string    `json:"username"`
	Phone                *string    `json:"phone"`
	Email                string     `json:"email"`
	EmailVerified        *time.Time `json:"emailVerified"`
	Password             string     `json:"-"`
	Image                *string    `json:"image"`
	CreatedAt            time.Time  `json:"createdAt"`
	UpdatedAt            time.Time  `json:"updatedAt"`
	MagicToken           *string    `json:"-"`
	MagicTokenValidUntil *time.Time `json:"-"`
	Referrals            *int       `json:"referrals"`
}

type UserOnTenant struct {
	UserID    string    `json:"userId"`
	TenantID  string    `json:"tenantId"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"createdAt"`
}

