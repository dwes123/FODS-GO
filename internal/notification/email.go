package notification

import (
	"fmt"
	"net/smtp"
	"os"
)

var (
	smtpHost     string
	smtpPort     string
	smtpUsername string
	smtpPassword string
	smtpFrom     string
	emailEnabled bool
)

// InitEmail reads SMTP config from environment variables.
// If not configured, email sending is silently skipped.
func InitEmail() {
	smtpHost = os.Getenv("SMTP_HOST")
	smtpPort = os.Getenv("SMTP_PORT")
	smtpUsername = os.Getenv("SMTP_USERNAME")
	smtpPassword = os.Getenv("SMTP_PASSWORD")
	smtpFrom = os.Getenv("SMTP_FROM")

	if smtpHost != "" && smtpPort != "" && smtpFrom != "" {
		emailEnabled = true
		fmt.Printf("Email notifications enabled (SMTP: %s:%s)\n", smtpHost, smtpPort)
	} else {
		fmt.Println("Email notifications disabled (SMTP not configured)")
	}
}

// SendEmail sends an email to the specified recipient.
// Runs in a goroutine to avoid blocking the caller.
func SendEmail(to, subject, body string) {
	if !emailEnabled || to == "" {
		return
	}

	go func() {
		msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/html; charset=\"UTF-8\"\r\n\r\n%s",
			smtpFrom, to, subject, body)

		var auth smtp.Auth
		if smtpUsername != "" {
			auth = smtp.PlainAuth("", smtpUsername, smtpPassword, smtpHost)
		}

		addr := smtpHost + ":" + smtpPort
		err := smtp.SendMail(addr, auth, smtpFrom, []string{to}, []byte(msg))
		if err != nil {
			fmt.Printf("Email send error (to: %s): %v\n", to, err)
		}
	}()
}
