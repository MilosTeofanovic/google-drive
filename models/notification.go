package models

import "time"

type Notification struct {
	ID        string    `json:"id" bson:"id"`
	UserID    string    `json:"user_id" bson:"user_id"`
	Message   string    `json:"message" bson:"message"`
	Read      bool      `json:"read" bson:"read"`
	CreatedAt time.Time `json:"created_at" bson:"created_at"`
}
