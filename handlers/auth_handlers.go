package handlers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"drive-backend/models"
	"drive-backend/services"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

const (
	otpsTTL          = 5 * time.Minute
	resetTokenTTL    = 15 * time.Minute
	minPasswordAge   = 1 * time.Minute
	maxPasswordAge   = 2 * time.Minute
	loginLockoutTime = 5 * time.Minute
)

var (
	usersCollection  *mongo.Collection
	auditCollection  *mongo.Collection
	defaultUserQuota int64 = 100 * 1024 * 1024
	usersClient      *services.UsersClient
	filesClient      *services.FilesClient
	emailSender      *services.EmailSender
)

func InitUserCollection(col *mongo.Collection) {
	usersCollection = col
	if col != nil && col.Database() != nil {
		auditCollection = col.Database().Collection("audit_logs")
	}
}

func InitAuditCollection(col *mongo.Collection) {
	auditCollection = col
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

func InitEmailSender(sender *services.EmailSender) {
	emailSender = sender
}

func getEmailSender() *services.EmailSender {
	if emailSender == nil {
		emailSender = services.NewEmailSender()
	}
	return emailSender
}

// Za brzu simulaciju na odbrani postavi MIN_PASSWORD_AGE_MINUTES (npr. 1 ili 2) u .env
func getMinPasswordAge() time.Duration {
	if raw := strings.TrimSpace(os.Getenv("MIN_PASSWORD_AGE_MINUTES")); raw != "" {
		if mins, err := strconv.Atoi(raw); err == nil && mins > 0 {
			return time.Duration(mins) * time.Minute
		}
	}
	return minPasswordAge
}

// Za brzu simulaciju na odbrani postavi MAX_PASSWORD_AGE_MINUTES (npr. 1 ili 3) u .env
func getMaxPasswordAge() time.Duration {
	if raw := strings.TrimSpace(os.Getenv("MAX_PASSWORD_AGE_MINUTES")); raw != "" {
		if mins, err := strconv.Atoi(raw); err == nil && mins > 0 {
			return time.Duration(mins) * time.Minute
		}
	}
	return maxPasswordAge
}

// Trajanje privremene blokade prijave nakon isteka lozinke (podrazumevano 5 min, može preko LOGIN_LOCKOUT_MINUTES)
func getLoginLockoutDuration() time.Duration {
	if raw := strings.TrimSpace(os.Getenv("LOGIN_LOCKOUT_MINUTES")); raw != "" {
		if mins, err := strconv.Atoi(raw); err == nil && mins > 0 {
			return time.Duration(mins) * time.Minute
		}
	}
	return loginLockoutTime
}

func recordAuditEvent(ctx context.Context, userID primitive.ObjectID, email, action, reason, status, details string) {
	now := time.Now().UTC()
	entry := models.AuditLog{
		ID:        primitive.NewObjectID(),
		UserID:    userID,
		Email:     email,
		Action:    action,
		Reason:    reason,
		Status:    status,
		Details:   details,
		CreatedAt: now,
	}

	log.Printf("[AUDIT] time=%s email=%s action=%s status=%s reason=%s details=%s",
		now.Format(time.RFC3339), email, action, status, reason, details)

	if auditCollection != nil {
		auditCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = ctx
		if _, err := auditCollection.InsertOne(auditCtx, entry); err != nil {
			log.Printf("[AUDIT ERROR] Neuspešan upis u audit_logs: %v", err)
		}
	}
}

func generateOTPSCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

func generateRandomToken(numBytes int) (string, error) {
	b := make([]byte, numBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func validateStrongPassword(password string) error {
	if len(password) < 8 {
		return errors.New("Lozinka mora imati najmanje 8 karaktera")
	}

	var hasUpper, hasLower, hasDigit, hasSpecial bool
	for _, ch := range password {
		switch {
		case ch >= 'A' && ch <= 'Z':
			hasUpper = true
		case ch >= 'a' && ch <= 'z':
			hasLower = true
		case ch >= '0' && ch <= '9':
			hasDigit = true
		case strings.ContainsRune("!@#$%^&*()-_=+[]{}|;:,.<>?/~`", ch):
			hasSpecial = true
		}
	}

	if !hasUpper {
		return errors.New("Lozinka mora sadržati bar jedno veliko slovo")
	}
	if !hasLower {
		return errors.New("Lozinka mora sadržati bar jedno malo slovo")
	}
	if !hasDigit {
		return errors.New("Lozinka mora sadržati bar jedan broj")
	}
	if !hasSpecial {
		return errors.New("Lozinka mora sadržati bar jedan specijalan karakter (npr. !@#$%)")
	}

	return nil
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

	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	if req.Email == "" || req.Password == "" {
		http.Error(w, "Email i lozinka su obavezni", http.StatusBadRequest)
		return
	}

	if req.Password != req.ConfirmPassword {
		http.Error(w, "Lozinke se ne poklapaju", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := validateStrongPassword(req.Password); err != nil {
		recordAuditEvent(ctx, primitive.NilObjectID, req.Email, "REGISTER", "WEAK_PASSWORD", "DENIED", err.Error())
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var existing models.User
	err := usersCollection.FindOne(ctx, bson.M{"email": req.Email}).Decode(&existing)
	if err == nil {
		http.Error(w, "Korisnik sa ovim email-om već postoji", http.StatusConflict)
		return
	}

	now := time.Now().UTC()
	newUser := models.User{
		ID:                primitive.NewObjectID(),
		FirstName:         req.FirstName,
		LastName:          req.LastName,
		Email:             req.Email,
		Password:          req.Password,
		Role:              "NK",
		Quota:             defaultUserQuota,
		PasswordChangedAt: now,
		PasswordExpired:   false,
	}

	if _, err := usersCollection.InsertOne(ctx, newUser); err != nil {
		http.Error(w, "Greška pri upisu korisnika u bazu", http.StatusInternalServerError)
		return
	}

	recordAuditEvent(ctx, newUser.ID, newUser.Email, "REGISTER", "NEW_ACCOUNT", "SUCCESS", "Korisnik uspešno registrovan")

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Uspešna registracija!",
		"user_id": newUser.ID.Hex(),
	})
}

// Korak 1 prijave: provera email + lozinka, provera isteka roka lozinke (max 60 dana / simulacija) i slanje OTPS koda
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

	email := strings.TrimSpace(strings.ToLower(req.Email))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var user models.User
	err := usersCollection.FindOne(ctx, bson.M{
		"email":    email,
		"password": req.Password,
	}).Decode(&user)

	if err != nil {
		recordAuditEvent(ctx, primitive.NilObjectID, email, "LOGIN", "INVALID_CREDENTIALS", "DENIED", "Pogrešan email ili lozinka")
		http.Error(w, "Pogrešan email ili lozinka", http.StatusUnauthorized)
		return
	}

	now := time.Now().UTC()

	// Ako je nalog već privremeno blokiran zbog isteka lozinke
	if !user.LoginBlockedUntil.IsZero() && now.Before(user.LoginBlockedUntil) {
		rem := user.LoginBlockedUntil.Sub(now).Round(time.Second)
		details := fmt.Sprintf("Prijava onemogućena do %s (preostalo: %s); potreban reset lozinke",
			user.LoginBlockedUntil.Format(time.RFC3339), rem)
		recordAuditEvent(ctx, user.ID, user.Email, "LOGIN", "PASSWORD_EXPIRED_LOCKOUT", "BLOCKED", details)
		http.Error(
			w,
			fmt.Sprintf("Lozinka je istekla — prijava je privremeno onemogućena još %s. Koristite 'Zaboravili ste lozinku?' za reset.", rem),
			http.StatusForbidden,
		)
		return
	}

	// Provera maksimalnog roka važenja lozinke (podrazumevano 60 dana, preko MAX_PASSWORD_AGE_MINUTES kraće za odbranu)
	maxAge := getMaxPasswordAge()
	passwordAge := now.Sub(user.PasswordChangedAt)
	if !user.PasswordChangedAt.IsZero() && passwordAge > maxAge {
		lockUntil := now.Add(getLoginLockoutDuration())
		_, _ = usersCollection.UpdateOne(
			ctx,
			bson.M{"_id": user.ID},
			bson.M{"$set": bson.M{
				"password_expired":    true,
				"login_blocked_until": lockUntil,
			}},
		)

		details := fmt.Sprintf("Starost lozinke: %s > dozvoljeno: %s; blokada do: %s",
			passwordAge.Round(time.Second), maxAge.Round(time.Second), lockUntil.Format(time.RFC3339))
		recordAuditEvent(ctx, user.ID, user.Email, "LOGIN", "PASSWORD_EXPIRED", "BLOCKED", details)

		http.Error(
			w,
			fmt.Sprintf("Istekao je maksimalni rok važenja lozinke (%s). Prijava je privremeno onemogućena do %s — resetujte lozinku.",
				maxAge.Round(time.Second), lockUntil.Format("15:04:05")),
			http.StatusForbidden,
		)
		return
	}

	code, err := generateOTPSCode()
	if err != nil {
		http.Error(w, "Greška pri generisanju OTPS koda", http.StatusInternalServerError)
		return
	}

	expiresAt := now.Add(otpsTTL)
	_, err = usersCollection.UpdateOne(
		ctx,
		bson.M{"_id": user.ID},
		bson.M{
			"$set": bson.M{
				"otps_code":        code,
				"otps_expires_at":  expiresAt,
				"password_expired": false,
			},
			"$unset": bson.M{
				"login_blocked_until": "",
			},
		},
	)
	if err != nil {
		http.Error(w, "Greška pri čuvanju OTPS koda", http.StatusInternalServerError)
		return
	}

	getEmailSender().SendOTPS(user.Email, code)
	recordAuditEvent(ctx, user.ID, user.Email, "LOGIN_STEP_1", "OTPS_SENT", "PENDING_OTPS", "Poslat OTPS kod")

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"requires_otps": true,
		"email":         user.Email,
		"message":       "Jednokratna lozinka (OTPS) je poslata na vašu imejl adresu.",
	})
}

// Korak 2 prijave: potvrda OTPS koda i izdavanje tokena
func VerifyOTPSHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Dozvoljen je samo POST metod", http.StatusMethodNotAllowed)
		return
	}

	var req models.VerifyOTPSRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Nevalidan JSON format", http.StatusBadRequest)
		return
	}

	email := strings.TrimSpace(strings.ToLower(req.Email))
	code := strings.TrimSpace(req.Code)
	if email == "" || code == "" {
		http.Error(w, "Email i OTPS kod su obavezni", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var user models.User
	if err := usersCollection.FindOne(ctx, bson.M{"email": email}).Decode(&user); err != nil {
		http.Error(w, "Korisnik nije pronađen", http.StatusUnauthorized)
		return
	}

	now := time.Now().UTC()
	if user.OTPSCode == "" || user.OTPSCode != code || now.After(user.OTPSExpiresAt) {
		recordAuditEvent(ctx, user.ID, user.Email, "LOGIN_VERIFY_OTPS", "INVALID_OR_EXPIRED_OTPS", "DENIED", "Neuspešna OTPS potvrda")
		http.Error(w, "Nevalidan ili istekao OTPS kod", http.StatusUnauthorized)
		return
	}

	_, _ = usersCollection.UpdateOne(
		ctx,
		bson.M{"_id": user.ID},
		bson.M{"$unset": bson.M{
			"otps_code":       "",
			"otps_expires_at": "",
		}},
	)

	role := user.Role
	if role == "" {
		role = "NK"
	}

	recordAuditEvent(ctx, user.ID, user.Email, "LOGIN_SUCCESS", "OTPS_VERIFIED", "SUCCESS", "Uspešna prijava")

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Uspešna prijava!",
		"role":    role,
		"token":   user.ID.Hex(),
	})
}

// Promena lozinke: lozinka mora biti bar 1 dan (24h) stara (ili MIN_PASSWORD_AGE_MINUTES pri simulaciji)
func ChangePasswordHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Dozvoljen je samo POST metod", http.StatusMethodNotAllowed)
		return
	}

	userID := r.Header.Get("X-User-ID")
	if userID == "" {
		http.Error(w, "Neautorizovan pristup", http.StatusUnauthorized)
		return
	}

	objID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		http.Error(w, "Nevalidan ID korisnika", http.StatusBadRequest)
		return
	}

	var req models.ChangePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Nevalidan JSON format", http.StatusBadRequest)
		return
	}

	if req.NewPassword == "" || req.NewPassword != req.ConfirmPassword {
		http.Error(w, "Nova lozinka i potvrda lozinke moraju biti iste i neprazne", http.StatusBadRequest)
		return
	}

	if err := validateStrongPassword(req.NewPassword); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var user models.User
	if err := usersCollection.FindOne(ctx, bson.M{"_id": objID}).Decode(&user); err != nil {
		http.Error(w, "Korisnik nije pronađen", http.StatusNotFound)
		return
	}

	if user.Password != req.CurrentPassword {
		recordAuditEvent(ctx, user.ID, user.Email, "CHANGE_PASSWORD", "WRONG_CURRENT_PASSWORD", "DENIED", "Pogrešna trenutna lozinka")
		http.Error(w, "Trenutna lozinka nije ispravna", http.StatusUnauthorized)
		return
	}

	now := time.Now().UTC()
	minAge := getMinPasswordAge()

	// Ako stari korisnik nema zabeležen password_changed_at, postavimo početno vreme na sada
	if user.PasswordChangedAt.IsZero() {
		_, _ = usersCollection.UpdateOne(
			ctx,
			bson.M{"_id": user.ID},
			bson.M{"$set": bson.M{"password_changed_at": now}},
		)
		user.PasswordChangedAt = now
	}

	age := now.Sub(user.PasswordChangedAt)
	if age < minAge {
		remaining := minAge - age
		details := fmt.Sprintf("Starost lozinke: %s < minimum: %s (preostalo: %s)",
			age.Round(time.Second), minAge.Round(time.Second), remaining.Round(time.Second))
		recordAuditEvent(ctx, user.ID, user.Email, "CHANGE_PASSWORD", "PASSWORD_TOO_YOUNG", "DENIED", details)
		http.Error(
			w,
			fmt.Sprintf("Lozinka može da se promeni tek nakon što bude bar %s stara (preostalo: %s)",
				minAge.Round(time.Second), remaining.Round(time.Second)),
			http.StatusForbidden,
		)
		return
	}

	_, err = usersCollection.UpdateOne(
		ctx,
		bson.M{"_id": user.ID},
		bson.M{
			"$set": bson.M{
				"password":            req.NewPassword,
				"password_changed_at": now,
				"password_expired":    false,
			},
			"$unset": bson.M{
				"login_blocked_until": "",
			},
		},
	)
	if err != nil {
		http.Error(w, "Greška pri promeni lozinke", http.StatusInternalServerError)
		return
	}

	recordAuditEvent(ctx, user.ID, user.Email, "CHANGE_PASSWORD", "MIN_AGE_SATISFIED", "SUCCESS", "Lozinka uspešno promenjena")

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Lozinka je uspešno promenjena!",
	})
}

// Zahtev za reset lozinke kratkotrajnim linkom na poznatu imejl adresu
func ForgotPasswordHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Dozvoljen je samo POST metod", http.StatusMethodNotAllowed)
		return
	}

	var req models.ForgotPasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Nevalidan JSON format", http.StatusBadRequest)
		return
	}

	email := strings.TrimSpace(strings.ToLower(req.Email))
	if email == "" {
		http.Error(w, "Imejl adresa je obavezna", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var user models.User
	if err := usersCollection.FindOne(ctx, bson.M{"email": email}).Decode(&user); err != nil {
		http.Error(w, "Korisnik sa unetom imejl adresom ne postoji", http.StatusNotFound)
		return
	}

	token, err := generateRandomToken(24)
	if err != nil {
		http.Error(w, "Greška pri generisanju reset tokena", http.StatusInternalServerError)
		return
	}

	expiresAt := time.Now().UTC().Add(resetTokenTTL)
	_, err = usersCollection.UpdateOne(
		ctx,
		bson.M{"_id": user.ID},
		bson.M{"$set": bson.M{
			"reset_token":      token,
			"reset_expires_at": expiresAt,
		}},
	)
	if err != nil {
		http.Error(w, "Greška pri čuvanju reset tokena", http.StatusInternalServerError)
		return
	}

	getEmailSender().SendPasswordResetLink(user.Email, token)
	recordAuditEvent(ctx, user.ID, user.Email, "FORGOT_PASSWORD", "RESET_LINK_SENT", "SUCCESS", "Poslat link za reset lozinke")

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Kratkotrajni link za reset lozinke je poslat na vašu imejl adresu.",
	})
}

// Potvrda reseta lozinke preko kratkotrajnog tokena/linka
func ResetPasswordHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Dozvoljen je samo POST metod", http.StatusMethodNotAllowed)
		return
	}

	var req models.ResetPasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Nevalidan JSON format", http.StatusBadRequest)
		return
	}

	token := strings.TrimSpace(req.Token)
	if token == "" || req.NewPassword == "" {
		http.Error(w, "Token i nova lozinka su obavezni", http.StatusBadRequest)
		return
	}

	if req.NewPassword != req.ConfirmPassword {
		http.Error(w, "Lozinke se ne poklapaju", http.StatusBadRequest)
		return
	}

	if err := validateStrongPassword(req.NewPassword); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var user models.User
	if err := usersCollection.FindOne(ctx, bson.M{"reset_token": token}).Decode(&user); err != nil {
		http.Error(w, "Nevalidan token za reset lozinke", http.StatusBadRequest)
		return
	}

	now := time.Now().UTC()
	if now.After(user.ResetExpiresAt) {
		recordAuditEvent(ctx, user.ID, user.Email, "RESET_PASSWORD", "EXPIRED_RESET_TOKEN", "DENIED", "Istekao token za reset lozinke")
		http.Error(w, "Link/token za reset lozinke je istekao", http.StatusBadRequest)
		return
	}

	_, err := usersCollection.UpdateOne(
		ctx,
		bson.M{"_id": user.ID},
		bson.M{
			"$set": bson.M{
				"password":            req.NewPassword,
				"password_changed_at": now,
				"password_expired":    false,
			},
			"$unset": bson.M{
				"reset_token":         "",
				"reset_expires_at":    "",
				"login_blocked_until": "",
			},
		},
	)
	if err != nil {
		http.Error(w, "Greška pri resetovanju lozinke", http.StatusInternalServerError)
		return
	}

	recordAuditEvent(ctx, user.ID, user.Email, "RESET_PASSWORD", "PASSWORD_RESET", "SUCCESS", "Lozinka resetovana i blokada prijave uklonjena")

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Lozinka je uspešno resetovana! Možete se prijaviti.",
	})
}

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
