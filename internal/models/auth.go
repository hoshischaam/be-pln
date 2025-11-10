package models

type RegisterRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required,min=8"`
}

type LoginRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required"`
}

type AuthResponse struct {
	AccessToken string `json:"accessToken"`
	UserID      string `json:"userId"`
}

type ForgotPasswordRequest struct {
	Email string `json:"email" validate:"required,email"`
}

type ResetPasswordRequest struct {
	TokenID      string `json:"tokenId"`                       // optional: ketika reset via email
	Token        string `json:"token"`                         // optional: raw token dari email
	UserID       string `json:"userId"`                        // optional: saat ganti password dari aplikasi
	OldPassword  string `json:"oldPassword"`                   // opsional: untuk change password
	NewPassword  string `json:"newPassword"`                   // password baru
	Password     string `json:"password"`                      // fallback untuk kompatibilitas lama
}
