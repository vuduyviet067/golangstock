package domain

import "time"

type WoodType struct {
	ID   int64
	Name string

	CreatedAt time.Time
	UpdatedAt time.Time
}
