package models

type Folder struct {
	ID       string  `json:"id" bson:"id"`
	Name     string  `json:"name" bson:"name"`
	ParentID *string `json:"parent_id,omitempty" bson:"parent_id,omitempty"` // nil ako je u root-u
	OwnerID  string  `json:"owner_id" bson:"owner_id"`
}
