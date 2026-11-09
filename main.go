package main

import (
	"encoding/json"
	"fmt"
	"net/http"
)

type User struct {
	ID              string `json:"id"`
	FirstName       string `json:"first_name"`
	LastName        string `json:"last_name"`
	Email           string `json:"email"`
	Password        string `json:"password"`
	ConfirmPassword string `json:"confirm_password"`
	Role            string `json:"role"`
	Quota           int64  `json:"quota"`
}

var users []User

func main() {
	http.HandleFunc("/api/register", registerHandler)
	http.HandleFunc("/api/login", loginHandler)
	http.HandleFunc("/api/change-email", changeEmailHandler)
	http.HandleFunc("/api/change-password", changePasswordHandler)

	fmt.Println("Server je pokrenut na portu 8080...")
	err := http.ListenAndServe(":8080", nil)
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

	var newUser User
	err := json.NewDecoder(r.Body).Decode(&newUser)
	if err != nil {
		http.Error(w, "Nevalidan JSON format", http.StatusBadRequest)
		return
	}

	if newUser.Password != newUser.ConfirmPassword {
		http.Error(w, "Lozinke se ne poklapaju", http.StatusBadRequest)
		return
	}

	for _, u := range users {
		if u.Email == newUser.Email {
			http.Error(w, "Korisnik sa ovim email-om već postoji", http.StatusConflict)
			return
		}
	}

	newUser.Role = "RK"
	newUser.Quota = 100 * 1024 * 1024
	newUser.ID = fmt.Sprintf("%d", len(users)+1)

	users = append(users, newUser)

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Uspešna registracija!",
		"user_id": newUser.ID,
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

// change email handler
func changeEmailHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Dozvoljen je samo POST metod", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Email        string `json:"email"`
		Password     string `json:"password"`
		NewEmail     string `json:"new_email"`
		ConfirmEmail string `json:"confirm_new_email"`
	}

	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, "Nevalidan JSON format", http.StatusBadRequest)
		return
	}

	if req.NewEmail != req.ConfirmEmail {
		http.Error(w, "Novi email-ovi se ne poklapaju", http.StatusBadRequest)
		return
	}

	for _, u := range users {
		if u.Email == req.NewEmail {
			http.Error(w, "Korisnik sa ovim novim email-om već postoji", http.StatusConflict)
			return
		}
	}

	found := false
	for i := range users {
		if users[i].Email == req.Email && users[i].Password == req.Password {
			users[i].Email = req.NewEmail
			found = true
			break
		}
	}

	if !found {
		http.Error(w, "Pogrešan trenutni email ili lozinka", http.StatusUnauthorized)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Email je uspešno promenjen!",
	})
}

// change Password handler
func changePasswordHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Dozvoljen je samo POST metod", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Email           string `json:"email"`
		OldPassword     string `json:"old_password"`
		NewPassword     string `json:"new_password"`
		ConfirmPassword string `json:"confirm_new_password"`
	}

	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, "Nevalidan JSON format", http.StatusBadRequest)
		return
	}

	if req.NewPassword != req.ConfirmPassword {
		http.Error(w, "Nove lozinke se ne poklapaju", http.StatusBadRequest)
		return
	}

	found := false
	for i := range users {
		if users[i].Email == req.Email && users[i].Password == req.OldPassword {
			users[i].Password = req.NewPassword
			found = true
			break
		}
	}

	if !found {
		http.Error(w, "Pogrešan email ili stara lozinka", http.StatusUnauthorized)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Lozinka je uspešno promenjena!",
	})
}
