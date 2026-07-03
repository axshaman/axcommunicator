package handlers

import (
	"axcommutator/app/config"
	"axcommutator/app/db"
	"axcommutator/app/utils"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// === структуры ===

type ProjectOrder struct {
	FullName         string `json:"fullName"`
	CompanyName      string `json:"companyName,omitempty"`
	Country          string `json:"country,omitempty"`
	Address          string `json:"address,omitempty"`
	ContactInfo      string `json:"contactInfo"`
	ProjectLink      string `json:"projectLink,omitempty"`
	PaymentMethod    string `json:"paymentMethod,omitempty"`
	StartDate        string `json:"startDate,omitempty"`
	Language         string `json:"language,omitempty"`
	Feedback         string `json:"feedback,omitempty"`
	Subject          string `json:"subject,omitempty"`
	BriefFile        []byte `json:"briefFile,omitempty"`
	SpecificationPdf []byte `json:"specificationPdf,omitempty"`
	InvoicePdf       []byte `json:"invoicePdf,omitempty"`
	ContractPdf      []byte `json:"contractPdf,omitempty"`
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

// === кэши для антиспама и идемпотентности ===
var (
	recentRequests sync.Map
	requestCounter sync.Map
	maxCacheSize   = 5000
)

// === лимитер по IP ===
func allowRequest(ip string) bool {
	limiterIface, _ := requestCounter.LoadOrStore(ip, rate.NewLimiter(rate.Every(time.Minute/20), 20)) // 20 req/min
	limiter := limiterIface.(*rate.Limiter)
	return limiter.Allow()
}

// === основной обработчик ===
func HandleProjectOrder(w http.ResponseWriter, r *http.Request) {
	var order ProjectOrder
	if err := json.NewDecoder(r.Body).Decode(&order); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}
	if order.FullName == "" || order.ContactInfo == "" {
		respondWithError(w, http.StatusBadRequest, "Missing required fields")
		return
	}

	serviceName := r.Header.Get("X-Service-Name")
	if serviceName == "" {
		respondWithError(w, http.StatusBadRequest, "Missing X-Service-Name")
		return
	}

	clientIP := utils.GetRealIP(r)
	if !allowRequest(clientIP) {
		respondWithError(w, http.StatusTooManyRequests, "Too many requests from this IP")
		return
	}

	key := fmt.Sprintf("%s|%s|%s", clientIP, serviceName, order.ContactInfo)
	count := 0
	recentRequests.Range(func(_, _ any) bool {
		count++
		return count < maxCacheSize
	})
	if count >= maxCacheSize {
		respondWithError(w, http.StatusServiceUnavailable, "Server busy, try later")
		return
	}
	if _, exists := recentRequests.Load(key); exists {
		respondWithJSON(w, http.StatusOK, map[string]string{
			"status":  "duplicate",
			"message": "Request already processed recently",
		})
		return
	}
	recentRequests.Store(key, time.Now())
	time.AfterFunc(5*time.Minute, func() { recentRequests.Delete(key) })

	// === PDF валидация ===
	if len(order.SpecificationPdf) > 0 && !utils.ValidatePDF(order.SpecificationPdf) {
		respondWithError(w, http.StatusBadRequest, "Invalid specification PDF")
		return
	}
	if len(order.InvoicePdf) > 0 && !utils.ValidatePDF(order.InvoicePdf) {
		respondWithError(w, http.StatusBadRequest, "Invalid invoice PDF")
		return
	}
	if len(order.ContractPdf) > 0 && !utils.ValidatePDF(order.ContractPdf) {
		respondWithError(w, http.StatusBadRequest, "Invalid contract PDF")
		return
	}

	// === язык ===
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

	// === определение типа ===
	isOrder := order.PaymentMethod != "" ||
		len(order.SpecificationPdf) > 0 ||
		len(order.InvoicePdf) > 0 ||
		len(order.ContractPdf) > 0

	var filePaths map[string]string
	var err error
	if isOrder {
		filePaths, err = saveOrderFiles(order)
		if err != nil {
			respondWithError(w, http.StatusInternalServerError, "Failed to save files")
			return
		}
		defer cleanupFiles(filePaths)
	}

	// === данные для шаблонов ===
	templateData := map[string]interface{}{
		"full_name":    order.FullName,
		"company":      order.CompanyName,
		"contact":      order.ContactInfo,
		"project_link": order.ProjectLink,
		"payment":      order.PaymentMethod,
		"start_date":   order.StartDate,
		"language":     lang,
		"service_name": service.Name,
		"feedback":     order.Feedback,
		"subject":      order.Subject,
	}

	var subject, body string
	basePath := fmt.Sprintf("app/templates/%s", serviceName)

	if isOrder {
		subjectPath := filepath.Join(basePath, "subject_"+lang+".txt")
		bodyPath := filepath.Join(basePath, "email_order_"+lang+".txt")
		subject = utils.LoadTemplateOrDefault(subjectPath, "Order Confirmation", templateData)
		body = utils.LoadTemplateOrDefault(bodyPath, "Order received successfully.", templateData)
	} else {
		subjectPath := filepath.Join(basePath, "subject_"+lang+".txt")
		bodyPath := filepath.Join(basePath, "email_feedback_"+lang+".txt")
		subject = utils.LoadTemplateOrDefault(subjectPath, "New feedback from {full_name}", templateData)
		body = utils.LoadTemplateOrDefault(bodyPath,
			"📬 Feedback from {full_name}\n💬 {feedback}\n🏢 {company}\n📧 {contact}\n📞 {project_link}",
			templateData)
	}

	// === отправка ===
	attachments, _ := utils.PrepareAttachments(filePaths)
	_ = utils.SendOrderEmail(service, subject, body, order.ContactInfo, attachments)
	_ = utils.SendTelegramNotification(service, lang, templateData)
	_ = logOrderToDB(r, serviceName, lang, order)

	respondWithJSON(w, http.StatusOK, map[string]string{
		"status":  "success",
		"service": serviceName,
		"lang":    lang,
		"type":    func() string { if isOrder { return "order" } else { return "feedback" } }(),
	})
}

// ==== helpers ====

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
		if len(content) == 0 {
			continue
		}
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
func HandleCookieConsent(w http.ResponseWriter, r *http.Request) {
	var consent CookieConsent
	if err := json.NewDecoder(r.Body).Decode(&consent); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}
	if consent.ServiceName == "" || consent.Fingerprint == "" || consent.Timestamp == "" {
		respondWithError(w, http.StatusBadRequest, "Missing required fields")
		return
	}
	if err := logConsentToDB(consent); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to log consent")
		return
	}
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
// HealthCheck — проверка состояния сервиса
func HealthCheck(w http.ResponseWriter, r *http.Request) {
	respondWithJSON(w, http.StatusOK, map[string]string{
		"status": "healthy",
		"time":   time.Now().UTC().Format(time.RFC3339),
	})
}