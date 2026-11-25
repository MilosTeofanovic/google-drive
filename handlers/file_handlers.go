package handlers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"drive-backend/models"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

var (
	uploadDir            = "./uploads"
	maxUploadBytes int64 = 50 << 20
)

func InitUploadConfig(dir string, maxBytes int64) {
	if dir != "" {
		uploadDir = dir
	}
	if maxBytes > 0 {
		maxUploadBytes = maxBytes
	}
}

func UploadFileHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Dozvoljen je samo POST metod", http.StatusMethodNotAllowed)
		return
	}

	ownerID := r.Header.Get("X-User-ID")
	if ownerID == "" {
		http.Error(w, "Neautorizovan pristup", http.StatusUnauthorized)
		return
	}

	if err := r.ParseMultipartForm(maxUploadBytes); err != nil {
		http.Error(w, "Fajl je prevelik ili forma nije validna", http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "Greška pri čitanju fajla", http.StatusBadRequest)
		return
	}
	defer file.Close()

	if err := os.MkdirAll(uploadDir, os.ModePerm); err != nil {
		http.Error(w, "Greška na serveru pri kreiranju direktorijuma", http.StatusInternalServerError)
		return
	}

	fileID := primitive.NewObjectID().Hex()
	savePath := filepath.Join(uploadDir, fileID+"_"+header.Filename)

	dst, err := os.Create(savePath)
	if err != nil {
		http.Error(w, "Greška pri čuvanju fajla", http.StatusInternalServerError)
		return
	}
	defer dst.Close()

	writtenBytes, err := io.Copy(dst, file)
	if err != nil {
		http.Error(w, "Greška pri upisu sadržaja fajla", http.StatusInternalServerError)
		return
	}

	var folderID *string
	rawFolderID := r.FormValue("folder_id")
	if rawFolderID != "" && rawFolderID != "null" {
		folderID = &rawFolderID
	}

	newFile := models.File{
		ID:       fileID,
		Name:     header.Filename,
		FolderID: folderID,
		OwnerID:  ownerID,
		Size:     writtenBytes,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := fileCollection.InsertOne(ctx, newFile); err != nil {
		http.Error(w, "Greška pri upisu fajla u bazu", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "Fajl uspešno otpremljen!",
		"file":    newFile,
	})
}

func OpenFileHandler(w http.ResponseWriter, r *http.Request) {
	fileID := r.URL.Query().Get("id")
	if fileID == "" {
		http.Error(w, "Nedostaje ID fajla", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var file models.File
	if err := fileCollection.FindOne(ctx, bson.M{"id": fileID}).Decode(&file); err != nil {
		http.Error(w, "Fajl nije pronađen", http.StatusNotFound)
		return
	}

	filePath := filepath.Join(uploadDir, file.ID+"_"+file.Name)
	w.Header().Set("Content-Disposition", "inline; filename=\""+file.Name+"\"")
	http.ServeFile(w, r, filePath)
}
