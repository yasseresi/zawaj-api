package dto

// CreateGuestRequest adds a guest. Status defaults to pending when omitted.
type CreateGuestRequest struct {
	FullName     string  `json:"full_name" binding:"required,max=120"`
	Contact      string  `json:"contact" binding:"max=120"`
	Relationship string  `json:"relationship" binding:"max=80"`
	Status       string  `json:"status" binding:"omitempty,oneof=pending confirmed declined"`
	Companions   *int    `json:"companions" binding:"omitempty,min=0,max=1000"`
	TableLabel   string  `json:"table_label" binding:"max=40"`
	Meal         string  `json:"meal" binding:"max=40"`
	Note         *string `json:"note" binding:"omitempty,max=2000"` // optional first note
}

// UpdateGuestRequest edits a guest; all fields optional.
type UpdateGuestRequest struct {
	FullName     *string `json:"full_name" binding:"omitempty,max=120"`
	Contact      *string `json:"contact" binding:"omitempty,max=120"`
	Relationship *string `json:"relationship" binding:"omitempty,max=80"`
	Status       *string `json:"status" binding:"omitempty,oneof=pending confirmed declined"`
	Companions   *int    `json:"companions" binding:"omitempty,min=0,max=1000"`
	TableLabel   *string `json:"table_label" binding:"omitempty,max=40"`
	Meal         *string `json:"meal" binding:"omitempty,max=40"`
}

// SetStatusRequest changes only a guest's RSVP status.
type SetStatusRequest struct {
	Status string `json:"status" binding:"required,oneof=pending confirmed declined"`
}

// AddNoteRequest adds a note to a guest.
type AddNoteRequest struct {
	Body string `json:"body" binding:"required,max=2000"`
}
