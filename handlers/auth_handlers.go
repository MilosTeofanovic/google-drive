package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"drive-backend/models"
	"drive-backend/services"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

var (
	usersCollection  *mongo.Collection
	defaultUserQuota int64 = 100 * 1024 * 1024
	usersClient      *services.UsersClient
	filesClient      *services.FilesClient
)

func InitUserCollection(col *mongo.Collection) {
	usersCollection = col
}

func InitDefaultQuota(quotaBytes int64) {
	if quotaBytes > 0 {
		defaultUserQuota = quotaBytes
	}
}

func InitSyncClients(uClient *services.UsersClient, fClient *services.FilesClient) {
	usersClient = uClient
	filesClient = fClient
}

func RegisterHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Dozvoljen je samo POST metod", http.StatusMethodNotAllowed)
		return
	}

	var req models.RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Nevalidan JSON format", http.StatusBadRequest)
		return
	}

	if req.Password != req.ConfirmPassword {
		http.Error(w, "Lozinke se ne poklapaju", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var existing models.User
	err := usersCollection.FindOne(ctx, bson.M{"email": req.Email}).Decode(&existing)
	if err == nil {
		http.Error(w, "Korisnik sa ovim email-om već postoji", http.StatusConflict)
		return
	}

	newUser := models.User{
		ID:        primitive.NewObjectID(),
		FirstName: req.FirstName,
		LastName:  req.LastName,
		Email:     req.Email,
		Password:  req.Password,
		Role:      "RK",
		Quota:     defaultUserQuota,
	}

	if _, err := usersCollection.InsertOne(ctx, newUser); err != nil {
		http.Error(w, "Greška pri upisu korisnika u bazu", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Uspešna registracija!",
		"user_id": newUser.ID.Hex(),
	})
}

func LoginHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Dozvoljen je samo POST metod", http.StatusMethodNotAllowed)
		return
	}

	var req models.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Nevalidan JSON format", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var user models.User
	err := usersCollection.FindOne(ctx, bson.M{
		"email":    req.Email,
		"password": req.Password,
	}).Decode(&user)

	if err != nil {
		http.Error(w, "Pogrešan email ili lozinka", http.StatusUnauthorized)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Uspešna prijava!",
		"role":    user.Role,
		"token":   user.ID.Hex(),
	})
}

// InternalUserRouteHandler obrađuje interne sinhronizovane pozive ka Users servisu:
// GET /api/internal/users/{id}/quota i GET /api/internal/users/{id}/exists
func InternalUserRouteHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Dozvoljen je samo GET metod", http.StatusMethodNotAllowed)
		return
	}

	trimmed := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/internal/users/"), "/")
	parts := strings.Split(trimmed, "/")
	if len(parts) != 2 || parts[0] == "" {
		http.Error(w, "Nevalidna interna putanja", http.StatusBadRequest)
		return
	}

	userID := parts[0]
	action := parts[1]

	objID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		http.Error(w, "Nevalidan ID korisnika", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	switch action {
	case "quota":
		var user models.User
		if err := usersCollection.FindOne(ctx, bson.M{"_id": objID}).Decode(&user); err != nil {
			if err == mongo.ErrNoDocuments {
				http.Error(w, "Korisnik nije pronađen", http.StatusNotFound)
				return
			}
			http.Error(w, "Greška pri čitanju korisnika", http.StatusInternalServerError)
			return
		}

		quota := user.Quota
		if quota <= 0 {
			quota = defaultUserQuota
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(services.QuotaResponse{
			UserID: userID,
			Quota:  quota,
		})

	case "exists":
		count, err := usersCollection.CountDocuments(ctx, bson.M{"_id": objID})
		if err != nil {
			http.Error(w, "Greška pri proveri korisnika", http.StatusInternalServerError)
			return
		}
		if count == 0 {
			http.Error(w, "Korisnik nije pronađen", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(services.ExistsResponse{
			UserID: userID,
			Exists: true,
		})

	default:
		http.Error(w, "Nepoznata interna akcija", http.StatusNotFound)
	}
}

func GetQuotaInfoHandler(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("X-User-ID")
	if userID == "" {
		http.Error(w, "Neautorizovan pristup", http.StatusUnauthorized)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	quota, err := usersClient.GetUserQuota(ctx, userID)
	if err != nil {
		if errors.Is(err, services.ErrUserNotFound) {
			http.Error(w, "Korisnik nije pronađen", http.StatusNotFound)
			return
		}
		http.Error(w, "Greška pri dobavljanju kvote iz Users servisa", http.StatusBadGateway)
		return
	}

	usedBytes, err := filesClient.GetUsedStorage(ctx, userID)
	if err != nil {
		http.Error(w, "Greška pri izračunavanju zauzeća iz Files servisa", http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"user_id":         userID,
		"quota_bytes":     quota,
		"used_bytes":      usedBytes,
		"remaining_bytes": quota - usedBytes,
	})
}
