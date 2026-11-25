package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"drive-backend/models"
	"drive-backend/services"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

var folderCollection *mongo.Collection
var fileCollection *mongo.Collection

func InitFolderCollection(collection *mongo.Collection) {
	folderCollection = collection
}

func InitFileCollection(collection *mongo.Collection) {
	fileCollection = collection
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

	// Sinhrona provera da korisnik postoji kod Users servisa
	if usersClient != nil {
		existsCtx, cancelExists := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancelExists()

		if _, err := usersClient.UserExists(existsCtx, ownerID); err != nil {
			if errors.Is(err, services.ErrUserNotFound) {
				http.Error(w, "Korisnik ne postoji u Users servisu", http.StatusNotFound)
				return
			}
			http.Error(w, "Greška pri sinhronoj proveri korisnika", http.StatusBadGateway)
			return
		}
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

	// Provera da li već postoji folder sa istim nazivom za ovog korisnika na istom nivou
	filter := bson.M{
		"name":     req.Name,
		"owner_id": ownerID,
	}
	if req.ParentID == nil || *req.ParentID == "" {
		filter["parent_id"] = bson.M{"$in": []interface{}{nil, ""}}
	} else {
		filter["parent_id"] = *req.ParentID
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

// pregled sadrzaja foldera
func GetFolderContentHandler(w http.ResponseWriter, r *http.Request) {
	ownerID := r.Header.Get("X-User-ID")
	if ownerID == "" {
		ownerID = "1"
	}
	folderID := r.URL.Query().Get("folder_id")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var parentFilter interface{} = bson.M{"$in": []interface{}{nil, ""}}
	if folderID != "" {
		parentFilter = folderID
	}

	var subfolders []models.Folder
	cursorF, _ := folderCollection.Find(ctx, bson.M{"owner_id": ownerID, "parent_id": parentFilter})
	if cursorF != nil {
		cursorF.All(ctx, &subfolders)
	}

	var files []models.File
	if fileCollection != nil {
		cursorD, _ := fileCollection.Find(ctx, bson.M{"owner_id": ownerID, "folder_id": parentFilter})
		if cursorD != nil {
			cursorD.All(ctx, &files)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"subfolders": subfolders,
		"files":      files,
	})
}

// preimenovanje foldera ili fajla
func RenameItemHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodPut && r.Method != http.MethodPatch {
		http.Error(w, "Dozvoljeni su POST, PUT i PATCH metodi", http.StatusMethodNotAllowed)
		return
	}

	ownerID := r.Header.Get("X-User-ID")
	if ownerID == "" {
		ownerID = "1"
	}

	var req struct {
		Type    string `json:"type"`
		ID      string `json:"id"`
		NewName string `json:"new_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ID == "" || req.NewName == "" {
		http.Error(w, "Nevalidni podaci", http.StatusBadRequest)
		return
	}

	if req.Type != "folder" && req.Type != "file" {
		http.Error(w, "Tip mora biti 'folder' ili 'file'", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	if req.Type == "file" {
		var existingFile models.File
		if err := fileCollection.FindOne(ctx, bson.M{"id": req.ID, "owner_id": ownerID}).Decode(&existingFile); err != nil {
			http.Error(w, "Datoteka nije pronađena ili nemate pravo izmene", http.StatusNotFound)
			return
		}

		oldPath := filepath.Join(uploadDir, existingFile.ID+"_"+existingFile.Name)
		newPath := filepath.Join(uploadDir, existingFile.ID+"_"+req.NewName)
		if oldPath != newPath {
			if _, err := os.Stat(oldPath); err == nil {
				_ = os.Rename(oldPath, newPath)
			}
		}

		now := time.Now().UTC()
		res, err := fileCollection.UpdateOne(
			ctx,
			bson.M{"id": req.ID, "owner_id": ownerID},
			bson.M{"$set": bson.M{"name": req.NewName, "updated_at": now}},
		)
		if err != nil || res.MatchedCount == 0 {
			http.Error(w, "Greška pri preimenovanju datoteke", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"message":    "Naziv datoteke uspešno promenjen!",
			"id":         req.ID,
			"name":       req.NewName,
			"updated_at": now,
		})
		return
	}

	var targetFolder models.Folder
	if err := folderCollection.FindOne(ctx, bson.M{"id": req.ID, "owner_id": ownerID}).Decode(&targetFolder); err != nil {
		http.Error(w, "Folder nije pronađen ili nemate pravo izmene", http.StatusNotFound)
		return
	}

	dupFilter := bson.M{
		"name":     req.NewName,
		"owner_id": ownerID,
		"id":       bson.M{"$ne": req.ID},
	}
	if targetFolder.ParentID == nil || *targetFolder.ParentID == "" {
		dupFilter["parent_id"] = bson.M{"$in": []interface{}{nil, ""}}
	} else {
		dupFilter["parent_id"] = *targetFolder.ParentID
	}

	dupCount, err := folderCollection.CountDocuments(ctx, dupFilter)
	if err != nil {
		http.Error(w, "Greška prilikom provere naziva foldera", http.StatusInternalServerError)
		return
	}
	if dupCount > 0 {
		http.Error(w, "Folder sa tim nazivom već postoji na ovom nivou!", http.StatusConflict)
		return
	}

	res, err := folderCollection.UpdateOne(
		ctx,
		bson.M{"id": req.ID, "owner_id": ownerID},
		bson.M{"$set": bson.M{"name": req.NewName}},
	)
	if err != nil || res.MatchedCount == 0 {
		http.Error(w, "Greška pri preimenovanju foldera", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "Naziv foldera uspešno promenjen!",
		"id":      req.ID,
		"name":    req.NewName,
	})
}
