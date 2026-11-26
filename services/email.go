package services

import (
	"fmt"
	"log"
	"net/smtp"
	"os"
	"strings"
)

type EmailSender struct {
	host    string
	port    string
	user    string
	pass    string
	from    string
	baseURL string
}

func NewEmailSender() *EmailSender {
	port := os.Getenv("SMTP_PORT")
	if port == "" {
		port = "587"
	}
	baseURL := strings.TrimRight(os.Getenv("APP_BASE_URL"), "/")
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}
	from := os.Getenv("SMTP_FROM")
	if from == "" {
		from = os.Getenv("SMTP_USER")
	}
	if from == "" {
		from = "noreply@mojdrive.local"
	}

	return &EmailSender{
		host:    os.Getenv("SMTP_HOST"),
		port:    port,
		user:    os.Getenv("SMTP_USER"),
		pass:    os.Getenv("SMTP_PASS"),
		from:    from,
		baseURL: baseURL,
	}
}

func (e *EmailSender) SendOTPS(toEmail, code string) {
	subject := "Jednokratna lozinka (OTPS) - Moj Drive"
	body := fmt.Sprintf("Vaš jednokratni OTPS kod za prijavu je: %s (važi 5 minuta).", code)

	if e.host == "" {
		log.Printf("[OTPS DEV LOG] Za %s -> kod: %s", toEmail, code)
		return
	}

	if err := e.sendMail(toEmail, subject, body); err != nil {
		log.Printf("[OTPS FALLBACK LOG] Greška pri SMTP slanju za %s (%v) -> kod: %s", toEmail, err, code)
	}
}

func (e *EmailSender) SendPasswordResetLink(toEmail, token string) {
	resetLink := fmt.Sprintf("%s/?reset_token=%s", e.baseURL, token)
	subject := "Reset lozinke - Moj Drive"
	body := fmt.Sprintf("Link za reset lozinke (važi 15 minuta): %s\nToken: %s", resetLink, token)

	if e.host == "" {
		log.Printf("[RESET LINK DEV LOG] Za %s -> %s", toEmail, resetLink)
		return
	}

	if err := e.sendMail(toEmail, subject, body); err != nil {
		log.Printf("[RESET LINK FALLBACK LOG] Greška pri SMTP slanju za %s (%v) -> %s", toEmail, err, resetLink)
	}
}

func (e *EmailSender) sendMail(to, subject, body string) error {
	msg := []byte("From: " + e.from + "\r\n" +
		"To: " + to + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"Content-Type: text/plain; charset=UTF-8\r\n\r\n" +
		body + "\r\n")

	var auth smtp.Auth
	if e.user != "" {
		auth = smtp.PlainAuth("", e.user, e.pass, e.host)
	}

	addr := fmt.Sprintf("%s:%s", e.host, e.port)
	return smtp.SendMail(addr, auth, e.from, []string{to}, msg)
}
