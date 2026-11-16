package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"drive-backend/models"

	"go.mongodb.org/mongo-driver/bson"
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

	// Provera roditeljskog foldera ako je parent_id prosleđen i nije prazan
	if req.ParentID != nil && *req.ParentID != "" {
		var parentFolder models.Folder
		err := folderCollection.FindOne(ctx, bson.M{"id": *req.ParentID, "owner_id": ownerID}).Decode(&parentFolder)
		if err != nil {
			if err == mongo.ErrNoDocuments {
				http.Error(w, "Roditeljski folder ne postoji ili nemate pravo pristupa", http.StatusBadRequest)
				return
			}
			http.Error(w, "Greška prilikom provere roditeljskog foldera", http.StatusInternalServerError)
			return
		}
	}

	// Provera da li već postoji folder sa istim nazivom za ovog korisnika
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
		http.Error(w, "Folder sa tim nazivom već postoji!", http.StatusConflict)
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
