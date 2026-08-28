package dto

// RegisterDeviceRequest registers an FCM token for the current user's device.
type RegisterDeviceRequest struct {
	Token    string `json:"token" binding:"required,max=512"`
	Platform string `json:"platform" binding:"required,oneof=android ios web"`
}

// UnregisterDeviceRequest removes an FCM token (e.g. on logout).
type UnregisterDeviceRequest struct {
	Token string `json:"token" binding:"required,max=512"`
}
