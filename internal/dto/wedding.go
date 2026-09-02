package dto

// CreateWeddingRequest creates a new list. EventDate is an optional "YYYY-MM-DD".
type CreateWeddingRequest struct {
	Name        string  `json:"name" binding:"required,max=120"`
	EventDate   *string `json:"event_date" binding:"omitempty,datetime=2006-01-02"`
	Description string  `json:"description" binding:"max=2000"`
}

// UpdateWeddingRequest edits a list; all fields optional.
type UpdateWeddingRequest struct {
	Name        *string `json:"name" binding:"omitempty,max=120"`
	EventDate   *string `json:"event_date" binding:"omitempty,datetime=2006-01-02"`
	Description *string `json:"description" binding:"omitempty,max=2000"`
}

// CreateInviteLinkRequest generates a role-scoped share link. Role is editor|viewer.
type CreateInviteLinkRequest struct {
	Role string `json:"role" binding:"required,oneof=editor viewer"`
}

// InviteUserRequest invites a specific registered user (by username) to join a
// wedding with a role. Role is editor|viewer.
type InviteUserRequest struct {
	Username string `json:"username" binding:"required,max=120"`
	Role     string `json:"role" binding:"required,oneof=editor viewer"`
}

// SetRoleRequest changes a member's role.
type SetRoleRequest struct {
	Role string `json:"role" binding:"required,oneof=editor viewer"`
}

// TransferOwnershipRequest names the member who becomes the new owner.
type TransferOwnershipRequest struct {
	UserID string `json:"user_id" binding:"required,uuid"`
}
