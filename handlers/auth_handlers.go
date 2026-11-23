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

var usersCollection *mongo.Collection

func InitUserCollection(col *mongo.Collection) {
	usersCollection = col
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
		Quota:     100 * 1024 * 1024,
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
