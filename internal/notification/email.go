package notification

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
)

var (
	brevoAPIKey  string
	emailFrom    string
	emailEnabled bool
)

// InitEmail reads email config from environment variables.
// If not configured, email sending is silently skipped.
func InitEmail() {
	brevoAPIKey = os.Getenv("BREVO_API_KEY")
	emailFrom = os.Getenv("SMTP_FROM")

	if brevoAPIKey != "" && emailFrom != "" {
		emailEnabled = true
		fmt.Println("Email notifications enabled (Brevo HTTP API)")
	} else {
		fmt.Println("Email notifications disabled (BREVO_API_KEY not configured)")
	}
}

// SendEmail sends an email to the specified recipient via Brevo HTTP API.
// Runs in a goroutine to avoid blocking the caller.
func SendEmail(to, subject, body string) {
	if !emailEnabled || to == "" {
		return
	}

	go func() {
		payload := map[string]interface{}{
			"sender": map[string]string{
				"name":  "Front Office Dynasty Sports",
				"email": emailFrom,
			},
			"to": []map[string]string{
				{"email": to},
			},
			"subject":     subject,
			"htmlContent": body,
		}

		jsonBody, err := json.Marshal(payload)
		if err != nil {
			fmt.Printf("Email marshal error (to: %s): %v\n", to, err)
			return
		}

		req, err := http.NewRequest("POST", "https://api.brevo.com/v3/smtp/email", bytes.NewBuffer(jsonBody))
		if err != nil {
			fmt.Printf("Email request error (to: %s): %v\n", to, err)
			return
		}
		req.Header.Set("accept", "application/json")
		req.Header.Set("content-type", "application/json")
		req.Header.Set("api-key", brevoAPIKey)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			fmt.Printf("Email send error (to: %s): %v\n", to, err)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 300 {
			var result map[string]interface{}
			json.NewDecoder(resp.Body).Decode(&result)
			fmt.Printf("Email API error (to: %s): status=%d response=%v\n", to, resp.StatusCode, result)
		}
	}()
}
