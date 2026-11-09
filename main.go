package main

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// User predstavlja model korisnika prema tvojoj specifikaciji
type User struct {
	ID              string `json:"id"`
	FirstName       string `json:"first_name"`
	LastName        string `json:"last_name"`
	Email           string `json:"email"`
	Password        string `json:"password"`
	ConfirmPassword string `json:"confirm_password"`
	Role            string `json:"role"`
	Quota           int64  `json:"quota"` // npr. u bajtovima (npr. 1024 * 1024 * 100 za 100MB)
}

// In-memory baza (lista korisnika dok je aplikacija pokrenuta)
var users []User

func main() {
	// Definišemo rute
	http.HandleFunc("/api/register", registerHandler)
	http.HandleFunc("/api/login", loginHandler)

	fmt.Println("Server je pokrenut na portu 8080...")
	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		fmt.Printf("Greška pri pokretanju servera: %v\n", err)
	}
}

// 1. Handler za registraciju
func registerHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Dozvoljen je samo POST metod", http.StatusMethodNotAllowed)
		return
	}

	var newUser User
	err := json.NewDecoder(r.Body).Decode(&newUser)
	if err != nil {
		http.Error(w, "Nevalidan JSON format", http.StatusBadRequest)
		return
	}

	// Osnovne validacije iz specifikacije
	if newUser.Password != newUser.ConfirmPassword {
		http.Error(w, "Lozinke se ne poklapaju", http.StatusBadRequest)
		return
	}

	// Provera da li email već postoji
	for _, u := range users {
		if u.Email == newUser.Email {
			http.Error(w, "Korisnik sa ovim email-om već postoji", http.StatusConflict)
			return
		}
	}

	// Dodeljivanje uloge i podrazumevane kvote (npr. 100MB)
	newUser.Role = "RK"
	newUser.Quota = 100 * 1024 * 1024
	// Generišemo jednostavan ID (za sad na osnovu broja korisnika + 1)
	newUser.ID = fmt.Sprintf("%d", len(users)+1)

	// Čuvanje u memorijsku listu
	users = append(users, newUser)

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Uspešna registracija!",
		"user_id": newUser.ID,
	})
}

// 2. Handler za login
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

	// Traženje korisnika u memoriji
	for _, u := range users {
		if u.Email == credentials.Email && u.Password == credentials.Password {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{
				"message": "Uspešna prijava!",
				"role":    u.Role,
			})
			return
		}
	}

	http.Error(w, "Pogrešan email ili lozinka", http.StatusUnauthorized)
}
