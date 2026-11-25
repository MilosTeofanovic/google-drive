package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"drive-backend/models"
	"drive-backend/services"

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

	// Sinhrona provera kvote korisnika kod Users servisa pre čuvanja fajla
	quotaCtx, quotaCancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer quotaCancel()

	if usersClient != nil {
		quota, err := usersClient.GetUserQuota(quotaCtx, ownerID)
		if err != nil {
			if errors.Is(err, services.ErrUserNotFound) {
				http.Error(w, "Korisnik nije pronađen u Users servisu", http.StatusNotFound)
				return
			}
			http.Error(w, "Greška pri sinhronoj proveri kvote kod Users servisa", http.StatusBadGateway)
			return
		}

		usedBytes, err := calculateUsedStorage(quotaCtx, ownerID)
		if err != nil {
			http.Error(w, "Greška pri proveri trenutnog zauzeća prostora", http.StatusInternalServerError)
			return
		}

		if usedBytes+header.Size > quota {
			http.Error(w, "Prekoračena korisnička kvota za otpremanje datoteke", http.StatusRequestEntityTooLarge)
			return
		}
	}

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

	now := time.Now().UTC()
	newFile := models.File{
		ID:        fileID,
		Name:      header.Filename,
		FolderID:  folderID,
		OwnerID:   ownerID,
		Size:      writtenBytes,
		UpdatedAt: now,
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

func UploadNewFileVersionHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodPut {
		http.Error(w, "Dozvoljeni su samo POST i PUT metodi", http.StatusMethodNotAllowed)
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

	fileID := r.URL.Query().Get("id")
	if fileID == "" {
		fileID = r.FormValue("file_id")
	}
	if fileID == "" {
		http.Error(w, "Nedostaje ID postojeće datoteke (id ili file_id)", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	var existingFile models.File
	if err := fileCollection.FindOne(ctx, bson.M{"id": fileID}).Decode(&existingFile); err != nil {
		http.Error(w, "Datoteka nije pronađena", http.StatusNotFound)
		return
	}

	if existingFile.OwnerID != ownerID {
		http.Error(w, "Nemate dozvolu za izmenu ove datoteke", http.StatusForbidden)
		return
	}

	// Sinhrona provera kvote uzimajući u obzir zamenu postojećeg fajla
	if usersClient != nil {
		quota, err := usersClient.GetUserQuota(ctx, ownerID)
		if err != nil {
			if errors.Is(err, services.ErrUserNotFound) {
				http.Error(w, "Korisnik nije pronađen u Users servisu", http.StatusNotFound)
				return
			}
			http.Error(w, "Greška pri sinhronoj proveri kvote kod Users servisa", http.StatusBadGateway)
			return
		}

		usedBytes, err := calculateUsedStorage(ctx, ownerID)
		if err != nil {
			http.Error(w, "Greška pri proveri trenutnog zauzeća prostora", http.StatusInternalServerError)
			return
		}

		projectedUsed := usedBytes - existingFile.Size + header.Size
		if projectedUsed < 0 {
			projectedUsed = header.Size
		}
		if projectedUsed > quota {
			http.Error(w, "Prekoračena korisnička kvota za novu verziju datoteke", http.StatusRequestEntityTooLarge)
			return
		}
	}

	if err := os.MkdirAll(uploadDir, os.ModePerm); err != nil {
		http.Error(w, "Greška na serveru pri kreiranju direktorijuma", http.StatusInternalServerError)
		return
	}

	oldPath := filepath.Join(uploadDir, existingFile.ID+"_"+existingFile.Name)
	_ = os.Remove(oldPath)

	newPath := filepath.Join(uploadDir, existingFile.ID+"_"+header.Filename)
	dst, err := os.Create(newPath)
	if err != nil {
		http.Error(w, "Greška pri čuvanju nove verzije fajla", http.StatusInternalServerError)
		return
	}
	defer dst.Close()

	writtenBytes, err := io.Copy(dst, file)
	if err != nil {
		http.Error(w, "Greška pri upisu sadržaja nove verzije fajla", http.StatusInternalServerError)
		return
	}

	now := time.Now().UTC()
	update := bson.M{
		"$set": bson.M{
			"name":       header.Filename,
			"size":       writtenBytes,
			"updated_at": now,
		},
	}

	if _, err := fileCollection.UpdateOne(ctx, bson.M{"id": existingFile.ID, "owner_id": ownerID}, update); err != nil {
		http.Error(w, "Greška pri ažuriranju datoteke u bazi", http.StatusInternalServerError)
		return
	}

	existingFile.Name = header.Filename
	existingFile.Size = writtenBytes
	existingFile.UpdatedAt = now

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "Nova verzija datoteke je uspešno otpremljena!",
		"file":    existingFile,
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

func InternalUsedStorageHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Dozvoljen je samo GET metod", http.StatusMethodNotAllowed)
		return
	}

	userID := r.URL.Query().Get("user_id")
	if userID == "" {
		http.Error(w, "Nedostaje user_id", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	usedBytes, err := calculateUsedStorage(ctx, userID)
	if err != nil {
		http.Error(w, "Greška pri izračunavanju zauzeća", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(services.UsedStorageResponse{
		UserID:    userID,
		UsedBytes: usedBytes,
	})
}

func calculateUsedStorage(ctx context.Context, userID string) (int64, error) {
	if fileCollection == nil {
		return 0, nil
	}

	cursor, err := fileCollection.Find(ctx, bson.M{"owner_id": userID})
	if err != nil {
		return 0, err
	}
	defer cursor.Close(ctx)

	var files []models.File
	if err := cursor.All(ctx, &files); err != nil {
		return 0, err
	}

	var total int64
	for _, f := range files {
		total += f.Size
	}
	return total, nil
}
