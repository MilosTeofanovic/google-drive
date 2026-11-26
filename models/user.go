package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type User struct {
	ID                primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	FirstName         string             `bson:"first_name" json:"first_name"`
	LastName          string             `bson:"last_name" json:"last_name"`
	Email             string             `bson:"email" json:"email"`
	Password          string             `bson:"password" json:"password"`
	Role              string             `bson:"role" json:"role"`
	Quota             int64              `bson:"quota" json:"quota"`
	PasswordChangedAt time.Time          `bson:"password_changed_at,omitempty" json:"password_changed_at,omitempty"`
	PasswordExpired   bool               `bson:"password_expired,omitempty" json:"password_expired,omitempty"`
	LoginBlockedUntil time.Time          `bson:"login_blocked_until,omitempty" json:"login_blocked_until,omitempty"`
	OTPSCode          string             `bson:"otps_code,omitempty" json:"-"`
	OTPSExpiresAt     time.Time          `bson:"otps_expires_at,omitempty" json:"-"`
	ResetToken        string             `bson:"reset_token,omitempty" json:"-"`
	ResetExpiresAt    time.Time          `bson:"reset_expires_at,omitempty" json:"-"`
}

type AuditLog struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	UserID    primitive.ObjectID `bson:"user_id,omitempty" json:"user_id,omitempty"`
	Email     string             `bson:"email" json:"email"`
	Action    string             `bson:"action" json:"action"`
	Reason    string             `bson:"reason" json:"reason"`
	Status    string             `bson:"status" json:"status"`
	Details   string             `bson:"details,omitempty" json:"details,omitempty"`
	CreatedAt time.Time          `bson:"created_at" json:"created_at"`
}

type RegisterRequest struct {
	FirstName       string `json:"first_name"`
	LastName        string `json:"last_name"`
	Email           string `json:"email"`
	Password        string `json:"password"`
	ConfirmPassword string `json:"confirm_password"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type VerifyOTPSRequest struct {
	Email string `json:"email"`
	Code  string `json:"code"`
}

type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
	ConfirmPassword string `json:"confirm_password"`
}

type ForgotPasswordRequest struct {
	Email string `json:"email"`
}

type ResetPasswordRequest struct {
	Token           string `json:"token"`
	NewPassword     string `json:"new_password"`
	ConfirmPassword string `json:"confirm_password"`
}
