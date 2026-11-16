package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"drive-backend/models"

	"go.mongodb.org/mongo-driver/bson" // <--- Proveri da li imaš ovaj import
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

var folderCollection *mongo.Collection

func InitFolderCollection(collection *mongo.Collection) {
	folderCollection = collection
}

func CreateFolderHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Dozvoljen je samo POST metod", http.StatusMethodNotAllowed)
		return
	}

	ownerID := r.Header.Get("X-User-ID")
	if ownerID == "" {
		ownerID = "1" // Fallback za testiranje
	}

	var req struct {
		Name     string  `json:"name"`
		ParentID *string `json:"parent_id"`
	}

	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, "Nevalidan JSON format", http.StatusBadRequest)
		return
	}

	if req.Name == "" {
		http.Error(w, "Naziv foldera ne sme biti prazan", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	filter := bson.M{
		"name":     req.Name,
		"owner_id": ownerID,
	}

	count, err := folderCollection.CountDocuments(ctx, filter)
	if err != nil {
		http.Error(w, "Greška prilikom provere baze", http.StatusInternalServerError)
		return
	}

	if count > 0 {
		http.Error(w, "Folder sa tim nazivom već postoji!", http.StatusConflict) // Status 409 Conflict
		return
	}

	newFolder := models.Folder{
		ID:       primitive.NewObjectID().Hex(),
		Name:     req.Name,
		ParentID: req.ParentID,
		OwnerID:  ownerID,
	}

	_, err = folderCollection.InsertOne(ctx, newFolder)
	if err != nil {
		http.Error(w, "Greška pri čuvanju foldera u bazu", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "Folder je uspešno kreiran u bazi!",
		"folder":  newFolder,
	})
}
