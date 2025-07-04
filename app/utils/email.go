package utils

import (
	"axcommutator/app/config"
	"bytes"
	"encoding/base64"
	"fmt"
	"net/mail"
	"net/smtp"
	"path/filepath"
	"os"
	// "log"
)

// EmailAttachment represents an attachment in an email
type EmailAttachment struct {
	Name    string
	Content []byte
}

// SendOrderEmail sends a MIME email with optional attachments
func SendOrderEmail(service config.ServiceConfig, subject, body, recipient string, attachments []EmailAttachment) error {
	if _, err := mail.ParseAddress(recipient); err != nil {
		return fmt.Errorf("invalid recipient email: %w", err)
	}

	var msg bytes.Buffer
	boundary := "AXCOMMUTATOR-MIME-BOUNDARY"

	// === Headers ===
	msg.WriteString(fmt.Sprintf("From: %s\r\n", service.SMTP.From))
	msg.WriteString(fmt.Sprintf("To: %s\r\n", recipient))
	if service.SMTP.Admin != "" {
		msg.WriteString(fmt.Sprintf("Bcc: %s\r\n", service.SMTP.Admin))
	}
	msg.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
	msg.WriteString("MIME-Version: 1.0\r\n")
	msg.WriteString(fmt.Sprintf("Content-Type: multipart/mixed; boundary=%q\r\n", boundary))
	msg.WriteString("\r\n")

	// === Body Part ===
	msg.WriteString(fmt.Sprintf("--%s\r\n", boundary))
	msg.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	msg.WriteString("Content-Transfer-Encoding: 7bit\r\n")
	msg.WriteString("\r\n")
	msg.WriteString(body + "\r\n")

	// === Attachments ===
	for _, a := range attachments {
		msg.WriteString(fmt.Sprintf("--%s\r\n", boundary))
		msg.WriteString("Content-Type: application/pdf\r\n")
		msg.WriteString("Content-Transfer-Encoding: base64\r\n")
		msg.WriteString(fmt.Sprintf("Content-Disposition: attachment; filename=\"%s\"\r\n", a.Name))
		msg.WriteString("\r\n")

		encoded := base64.StdEncoding.EncodeToString(a.Content)
		for i := 0; i < len(encoded); i += 76 {
			end := i + 76
			if end > len(encoded) {
				end = len(encoded)
			}
			msg.WriteString(encoded[i:end] + "\r\n")
		}
	}

	// === End Boundary ===
	msg.WriteString(fmt.Sprintf("--%s--\r\n", boundary))

	// === Send Email ===
	smtpAddr := fmt.Sprintf("%s:%s", service.SMTP.Host, service.SMTP.Port)
	auth := smtp.PlainAuth("", service.SMTP.User, service.SMTP.Password, service.SMTP.Host)

	recipients := []string{recipient}
	if service.SMTP.Admin != "" {
		recipients = append(recipients, service.SMTP.Admin)
	}

	if err := smtp.SendMail(smtpAddr, auth, service.SMTP.From, recipients, msg.Bytes()); err != nil {
		return fmt.Errorf("SMTP send failed: %w", err)
	}

	return nil
}

// PrepareAttachments prepares email attachments from temporary files
func PrepareAttachments(filePaths map[string]string) ([]EmailAttachment, error) {
	var attachments []EmailAttachment

	for _, path := range filePaths {
		content, err := os.ReadFile(path)
		if err != nil {
			// log.Printf("Failed to read attachment %s: %v", path, err)
			continue
		}

		attachments = append(attachments, EmailAttachment{
			Name:    filepath.Base(path),
			Content: content,
		})
	}

	if len(attachments) == 0 {
		return nil, fmt.Errorf("no valid attachments found")
	}

	return attachments, nil
}

// ValidateEmail checks if an email address is valid
func ValidateEmail(email string) bool {
	_, err := mail.ParseAddress(email)
	return err == nil
}