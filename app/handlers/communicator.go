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
	Language 		string `json:"language"`
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
	var order ProjectOrder
	if err := json.NewDecoder(r.Body).Decode(&order); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}

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

	// определение языка: заголовок > тело > fallback
	lang := order.Language
	if lang == "" {
		lang = r.Header.Get("Accept-Language")
	}
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

	filePaths, err := saveOrderFiles(order)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to save files")
		return
	}
	defer cleanupFiles(filePaths)

	templateData := map[string]interface{}{
		"full_name":    order.FullName,
		"company":      order.CompanyName,
		"contact":      order.ContactInfo,
		"project_link": order.ProjectLink,
		"payment":      order.PaymentMethod,
		"start_date":   order.StartDate,
		"language":     lang,
		"service_name": service.Name,
	}

	subject := "Order Confirmation"
	body := "Order received VibeCoders Club by Aleksandr Shaman - www.codcl.com"

	// шаблон темы письма
	if path := service.EmailTemplateSubjectPaths[lang]; path != "" {
		if raw, err := os.ReadFile(path); err == nil {
			subject = utils.FillTemplate(string(raw), templateData)
		}
	}

	// шаблон тела письма
	if path := service.EmailTemplateBodyPaths[lang]; path != "" {
		if raw, err := os.ReadFile(path); err == nil {
			body = utils.FillTemplate(string(raw), templateData)
		}
	}

	attachments, err := utils.PrepareAttachments(filePaths)
	if err == nil {
		_ = utils.SendOrderEmail(service, subject, body, order.ContactInfo, attachments)
	}

	_ = utils.SendTelegramNotification(service, lang, templateData)
	_ = logOrderToDB(r, serviceName, lang, order)

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
		payment_method, start_date, language, ip_address, user_agent, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		serviceName,
		order.FullName,
		order.CompanyName,
		order.ContactInfo,
		order.ProjectLink,
		order.PaymentMethod,
		order.StartDate,
		order.Language,
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