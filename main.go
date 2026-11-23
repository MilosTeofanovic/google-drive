package main

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"time"

	"drive-backend/handlers"
	"drive-backend/middleware"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type User struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	FirstName string             `bson:"first_name" json:"first_name"`
	LastName  string             `bson:"last_name" json:"last_name"`
	Email     string             `bson:"email" json:"email"`
	Password  string             `bson:"password" json:"password"`
	Role      string             `bson:"role" json:"role"`
	Quota     int64              `bson:"quota" json:"quota"`
}

var usersCollection *mongo.Collection

func main() {
	// 1. Povezivanje sa MongoDB bazom
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	clientOptions := options.Client().ApplyURI("mongodb+srv://milosteofanovic2005_db_user:Milos12345@projekat.kxn9abn.mongodb.net/")
	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		log.Fatalf("Greška pri povezivanju sa MongoDB: %v", err)
	}

	// Provera konekcije
	err = client.Ping(ctx, nil)
	if err != nil {
		log.Fatalf("MongoDB nije dostupan: %v", err)
	}
	fmt.Println("Uspešno povezani sa MongoDB bazom!")

	// Inicijalizacija kolekcija u bazi
	db := client.Database("google_drive")
	handlers.InitFolderCollection(db.Collection("folders"))
	handlers.InitFileCollection(db.Collection("files"))
	usersCollection = db.Collection("users")

	// 2. Registracija ruta
	http.HandleFunc("/api/folders", middleware.AuthMiddleware(handlers.CreateFolderHandler))
	http.HandleFunc("/api/files/upload", middleware.AuthMiddleware(handlers.UploadFileHandler))
	http.HandleFunc("/api/files/open", handlers.OpenFileHandler)
	http.HandleFunc("/api/folders/content", middleware.AuthMiddleware(handlers.GetFolderContentHandler))
	http.HandleFunc("/api/items/rename", middleware.AuthMiddleware(handlers.RenameItemHandler))
	http.HandleFunc("/api/register", registerHandler)
	http.HandleFunc("/api/login", loginHandler)

	// Serviranje statičkih fajlova (CSS)
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("./static"))))

	// Ruta koja učitava HTML šablon
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		tmpl, err := template.ParseFiles("templates/index.html")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		tmpl.Execute(w, nil)
	})

	fmt.Println("Server je pokrenut na portu 8080...")
	err = http.ListenAndServe(":8080", nil)
	if err != nil {
		fmt.Printf("Greška pri pokretanju servera: %v\n", err)
	}
}

// register handler
func registerHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Dozvoljen je samo POST metod", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		FirstName       string `json:"first_name"`
		LastName        string `json:"last_name"`
		Email           string `json:"email"`
		Password        string `json:"password"`
		ConfirmPassword string `json:"confirm_password"`
	}

	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, "Nevalidan JSON format", http.StatusBadRequest)
		return
	}

	if req.Password != req.ConfirmPassword {
		http.Error(w, "Lozinke se ne poklapaju", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var existing User
	err = usersCollection.FindOne(ctx, bson.M{"email": req.Email}).Decode(&existing)
	if err == nil {
		http.Error(w, "Korisnik sa ovim email-om već postoji", http.StatusConflict)
		return
	}

	newUser := User{
		ID:        primitive.NewObjectID(),
		FirstName: req.FirstName,
		LastName:  req.LastName,
		Email:     req.Email,
		Password:  req.Password,
		Role:      "RK",
		Quota:     100 * 1024 * 1024,
	}

	_, err = usersCollection.InsertOne(ctx, newUser)
	if err != nil {
		http.Error(w, "Greška pri upisu korisnika u bazu", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Uspešna registracija!",
		"user_id": newUser.ID.Hex(),
	})
}

// login handler
func loginHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Dozvoljen je samo POST metod", http.StatusMethodNotAllowed)
		return
	}

	var credentials struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	err := json.NewDecoder(r.Body).Decode(&credentials)
	if err != nil {
		http.Error(w, "Nevalidan JSON format", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var u User
	err = usersCollection.FindOne(ctx, bson.M{
		"email":    credentials.Email,
		"password": credentials.Password,
	}).Decode(&u)

	if err != nil {
		http.Error(w, "Pogrešan email ili lozinka", http.StatusUnauthorized)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Uspešna prijava!",
		"role":    u.Role,
		"token":   u.ID.Hex(),
	})
}
