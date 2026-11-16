package models

type File struct {
	ID       string  `json:"id" bson:"id"`
	Name     string  `json:"name" bson:"name"`
	FolderID *string `json:"folder_id,omitempty" bson:"folder_id,omitempty"`
	OwnerID  string  `json:"owner_id" bson:"owner_id"`
	Size     int64   `json:"size" bson:"size"`
}
