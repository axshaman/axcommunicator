package handlers

import (
	"axcommutator/app/config"
	"axcommutator/app/db"
	"axcommutator/app/utils"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"
)

type ProjectOrder struct {
	FullName        string `json:"fullName"`
	CompanyName     string `json:"companyName,omitempty"`
	Country         string `json:"country,omitempty"`
	Address         string `json:"address,omitempty"`
	ContactInfo     string `json:"contactInfo"`
	ProjectLink     string `json:"projectLink,omitempty"`
	PaymentMethod   string `json:"paymentMethod"`
	StartDate       string `json:"startDate"`
	Languages       int    `json:"languages"`
	BriefFile        []byte `json:"briefFile,omitempty"`
	SpecificationPdf []byte `json:"specificationPdf"`
	InvoicePdf       []byte `json:"invoicePdf"`
	ContractPdf      []byte `json:"contractPdf"`
}

type CookieConsent struct {
	ServiceName string `json:"serviceName"`
	Fingerprint string `json:"fingerprint"`
	UserAgent   string `json:"userAgent"`
	IPAddress   string `json:"ipAddress"`
	Accepted    bool   `json:"accepted"`
	Timestamp   string `json:"timestamp"`
	Language    string `json:"language,omitempty"`
}

func HandleProjectOrder(w http.ResponseWriter, r *http.Request) {
	// fmt.Println("🚀 HandleProjectOrder started")

	var order ProjectOrder
	if err := json.NewDecoder(r.Body).Decode(&order); err != nil {
		// fmt.Printf("❌ JSON decode error: %v\n", err)
		respondWithError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}
	// fmt.Printf("📦 Received order: %+v\n", order)

	if order.FullName == "" || order.ContactInfo == "" || order.PaymentMethod == "" {
		respondWithError(w, http.StatusBadRequest, "Missing required fields")
		return
	}

	if !utils.ValidatePDF(order.SpecificationPdf) || !utils.ValidatePDF(order.InvoicePdf) || !utils.ValidatePDF(order.ContractPdf) {
		respondWithError(w, http.StatusBadRequest, "Invalid PDF files")
		return
	}

	serviceName := r.Header.Get("X-Service-Name")
	if serviceName == "" {
		respondWithError(w, http.StatusBadRequest, "Missing X-Service-Name")
		return
	}

	lang := r.Header.Get("Accept-Language")
	if lang == "" {
		lang = "en"
	}

	service, ok := config.GetService(serviceName)
	if !ok {
		respondWithError(w, http.StatusBadRequest, "Service not configured")
		return
	}

	if !utils.Contains(service.SupportedLangs, lang) {
		if len(service.SupportedLangs) > 0 {
			lang = service.SupportedLangs[0]
		} else {
			lang = "en"
		}
	}
	// fmt.Printf("🌍 Service: %s | Lang: %s\n", service.Name, lang)

	filePaths, err := saveOrderFiles(order)
	if err != nil {
		// fmt.Printf("❌ File save error: %v\n", err)
		respondWithError(w, http.StatusInternalServerError, "Failed to save files")
		return
	}
	defer cleanupFiles(filePaths)
	// fmt.Printf("📁 Files: %+v\n", filePaths)

	templateData := map[string]interface{}{
		"full_name":    order.FullName,
		"company":      order.CompanyName,
		"contact":      order.ContactInfo,
		"project_link": order.ProjectLink,
		"payment":      order.PaymentMethod,
		"start_date":   order.StartDate,
		"languages":    order.Languages,
		"service_name": service.Name,
	}

	subject := "Order Confirmation"
	body := "Order received VibeCoders Club by Aleksandr Shaman - www.codcl.com"

	// Fill subject from file
	if path := service.EmailTemplateSubjectPaths[lang]; path != "" {
		if raw, err := os.ReadFile(path); err == nil {
			subject = utils.FillTemplate(string(raw), templateData)
			if err != nil {
				// fmt.Printf("❌ Subject render error: %v\n", err)
			} else {
				// fmt.Printf("📧 Final subject:\n%s\n", subject)
			}
		} else {
			// fmt.Printf("❌ Failed to read subject file: %v\n", err)
		}
	}

	// Fill body from file
	if path := service.EmailTemplateBodyPaths[lang]; path != "" {
		if raw, err := os.ReadFile(path); err == nil {
			body = utils.FillTemplate(string(raw), templateData)
			if err != nil {
				// fmt.Printf("❌ Body render error: %v\n", err)
			} else {
				// fmt.Printf("📧 Final body:\n%s\n", body)
			}
		} else {
			// fmt.Printf("❌ Failed to read body file: %v\n", err)
		}
	}
	

	attachments, err := utils.PrepareAttachments(filePaths)
	if err != nil {
		// fmt.Printf("❌ Attachments error: %v\n", err)
	} else {
		if err := utils.SendOrderEmail(service, subject, body, order.ContactInfo, attachments); err != nil {
			// fmt.Printf("❌ Email send error: %v\n", err)
		} else {
			// fmt.Println("✅ Email sent")
		}
	}

	if err := utils.SendTelegramNotification(service, lang, templateData); err != nil {
		// fmt.Printf("❌ Telegram error: %v\n", err)
	} else {
		// fmt.Println("✅ Telegram sent")
	}

	if err := logOrderToDB(r, serviceName, lang, order); err != nil {
		// fmt.Printf("❌ DB log error: %v\n", err)
	} else {
		// fmt.Println("✅ Order saved to DB")
	}

	respondWithJSON(w, http.StatusOK, map[string]string{
		"status":  "success",
		"service": serviceName,
		"lang":    lang,
	})
}

// ==== helper functions ====

func respondWithError(w http.ResponseWriter, code int, message string) {
	respondWithJSON(w, code, map[string]string{"error": message})
}

func respondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(payload)
}

func saveOrderFiles(order ProjectOrder) (map[string]string, error) {
	filePaths := make(map[string]string)
	files := map[string][]byte{
		"specification": order.SpecificationPdf,
		"invoice":       order.InvoicePdf,
		"contract":      order.ContractPdf,
	}
	if len(order.BriefFile) > 0 {
		files["brief"] = order.BriefFile
	}
	for name, content := range files {
		fileInfo, err := utils.SaveTempFile(content, name)
		if err != nil {
			for _, path := range filePaths {
				_ = os.Remove(path)
			}
			return nil, fmt.Errorf("failed to save %s: %v", name, err)
		}
		filePaths[name] = fileInfo.Path
	}
	return filePaths, nil
}

func cleanupFiles(filePaths map[string]string) {
	for _, path := range filePaths {
		_ = os.Remove(path)
	}
}

func logOrderToDB(r *http.Request, serviceName, lang string, order ProjectOrder) error {
	db := db.GetDB()
	_, err := db.Exec(
		`INSERT INTO project_orders 
		(service_name, full_name, company_name, contact_info, project_link, 
		payment_method, start_date, languages, ip_address, user_agent, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		serviceName,
		order.FullName,
		order.CompanyName,
		order.ContactInfo,
		order.ProjectLink,
		order.PaymentMethod,
		order.StartDate,
		order.Languages,
		utils.GetRealIP(r),
		r.UserAgent(),
		time.Now().UTC(),
	)
	return err
}

func HealthCheck(w http.ResponseWriter, r *http.Request) {
	respondWithJSON(w, http.StatusOK, map[string]string{
		"status": "healthy",
		"time":   time.Now().UTC().Format(time.RFC3339),
	})
}

func HandleCookieConsent(w http.ResponseWriter, r *http.Request) {
	var consent CookieConsent
	if err := json.NewDecoder(r.Body).Decode(&consent); err != nil {
		// fmt.Printf("❌ Invalid consent payload: %v\n", err)
		respondWithError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}

	if consent.ServiceName == "" || consent.Fingerprint == "" || consent.Timestamp == "" {
		// fmt.Println("❌ Missing required consent fields")
		respondWithError(w, http.StatusBadRequest, "Missing required fields")
		return
	}

	/*service, ok := config.GetService(consent.ServiceName)
	if !ok {
		// fmt.Printf("❌ Unknown service for consent: %s\n", consent.ServiceName)
		respondWithError(w, http.StatusBadRequest, "Service not configured")
		return
	}*/

	if err := logConsentToDB(consent); err != nil {
		// fmt.Printf("❌ Failed to log consent: %v\n", err)
		respondWithError(w, http.StatusInternalServerError, "Failed to log consent")
		return
	}

	// fmt.Printf("✅ Consent logged for service: %s\n", service.Name)
	respondWithJSON(w, http.StatusOK, map[string]string{
		"status":  "logged",
		"service": consent.ServiceName,
	})
}

func logConsentToDB(consent CookieConsent) error {
	db := db.GetDB()
	_, err := db.Exec(
		`INSERT INTO cookie_consents 
		(service_name, fingerprint, user_agent, ip_address, accepted, timestamp)
		VALUES (?, ?, ?, ?, ?, ?)`,
		consent.ServiceName,
		consent.Fingerprint,
		consent.UserAgent,
		consent.IPAddress,
		consent.Accepted,
		consent.Timestamp,
	)
	return err
}