package domain

import "time"

type Supplier struct {
	ID   int64
	Name string

	// Phone is nil when the supplier has no phone number.
	Phone *string

	CreatedAt time.Time
	UpdatedAt time.Time
}
