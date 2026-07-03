package config

import (
	"fmt"
	"os"
	"strings"
)

type ServiceConfig struct {
	Name                       string
	SMTP                       SMTPConfig
	Telegram                   TelegramConfig
	EmailTemplates             map[string]EmailTemplate
	TelegramTemplates          map[string]string
	TelegramTemplatePaths      map[string]string
	EmailTemplateSubjectPaths  map[string]string
	EmailTemplateBodyPaths     map[string]string
	SupportedLangs             []string
}

type SMTPConfig struct {
	User     string
	Password string
	Host     string
	Port     string
	From     string
	Admin    string
}

type TelegramConfig struct {
	BotToken string
	ChatID   string
}

type EmailTemplate struct {
	Subject string
	Body    string
}

func (tc TelegramConfig) Configured() bool {
	return tc.BotToken != "" && tc.ChatID != ""
}

var services = map[string]ServiceConfig{}

func LoadServices() {
	for _, env := range os.Environ() {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := parts[0]
		val := parts[1]

		// ✅ поддержка как ZD_SERVICE_NAME, так и ZD_SERVICE_NAME_SERVICE_NAME
		if strings.HasSuffix(key, "_SERVICE_NAME") {
			prefix := strings.TrimSuffix(key, "_SERVICE_NAME")
			name := val

			loadSingleService(prefix, name)
		}
	}
}

func loadSingleService(prefix, name string) {
	langs := strings.Split(os.Getenv(prefix+"_LANGS"), ",")
	if len(langs) == 0 || (len(langs) == 1 && langs[0] == "") {
		langs = []string{"en"}
	}

	emailTemplates := loadEmailTemplates(prefix, name, langs)
	emailSubjectPaths := loadEmailSubjectPaths(prefix, name, langs)
	emailBodyPaths := loadEmailBodyPaths(prefix, name, langs)
	tgTemplates := loadTelegramTemplates(prefix, name, langs)
	tgPaths := loadTelegramPaths(prefix, name, langs)

	services[strings.ToLower(name)] = ServiceConfig{
		Name: name,
		SMTP: SMTPConfig{
			User:     os.Getenv(prefix + "_SMTP_USER"),
			Password: os.Getenv(prefix + "_SMTP_PASS"),
			Host:     os.Getenv(prefix + "_SMTP_HOST"),
			Port:     os.Getenv(prefix + "_SMTP_PORT"),
			From:     os.Getenv(prefix + "_FROM_EMAIL"),
			Admin:    os.Getenv(prefix + "_ADMIN_EMAIL"),
		},
		Telegram: TelegramConfig{
			BotToken: os.Getenv(prefix + "_TG_BOT_TOKEN"),
			ChatID:   os.Getenv(prefix + "_TG_CHAT_ID"),
		},
		EmailTemplates:             emailTemplates,
		EmailTemplateSubjectPaths:  emailSubjectPaths,
		EmailTemplateBodyPaths:     emailBodyPaths,
		TelegramTemplates:          tgTemplates,
		TelegramTemplatePaths:      tgPaths,
		SupportedLangs:             langs,
	}

	// fmt.Printf("✅ Loaded service: %s (prefix: %s)\n", name, prefix)
}

func GetService(name string) (ServiceConfig, bool) {
	svc, ok := services[strings.ToLower(name)]
	// if !ok {
	// 	fmt.Printf("❌ Service not found: %s\n", name)
	// } else {
	// 	fmt.Printf("✅ Service resolved: %s\n", name)
	// }
	return svc, ok
}

func loadEmailTemplates(prefix string, service string, langs []string) map[string]EmailTemplate {
	templates := make(map[string]EmailTemplate)

	for _, lang := range langs {
		lang = strings.TrimSpace(strings.ToLower(lang))
		subjectKey := fmt.Sprintf("%s_EMAIL_SUBJECT_%s", prefix, strings.ToUpper(lang))
		bodyKey := fmt.Sprintf("%s_EMAIL_BODY_%s", prefix, strings.ToUpper(lang))
		bodyPathKey := bodyKey + "_PATH"

		subject := os.Getenv(subjectKey)
		body := os.Getenv(bodyKey)

		if body == "" {
			path := os.Getenv(bodyPathKey)
			if path != "" {
				content, err := os.ReadFile(path)
				if err != nil {
					// fmt.Printf("⚠️ [%s:%s] Failed to load email body from file: %v\n", service, lang, err)
				} else {
					body = string(content)
					// fmt.Printf("📄 [%s:%s] Loaded email body from %s\n", service, lang, path)
				}
			} else {
				// fmt.Printf("⚠️ [%s:%s] No body or path provided for email\n", service, lang)
			}
		} else {
			// fmt.Printf("📝 [%s:%s] Loaded email body from env\n", service, lang)
		}

		templates[lang] = EmailTemplate{
			Subject: subject,
			Body:    body,
		}
	}
	return templates
}

func loadTelegramTemplates(prefix string, service string, langs []string) map[string]string {
	templates := make(map[string]string)

	for _, lang := range langs {
		lang = strings.TrimSpace(strings.ToLower(lang))
		key := fmt.Sprintf("%s_TG_MSG_%s_PATH", prefix, strings.ToUpper(lang))
		path := os.Getenv(key)

		if path != "" {
			content, err := os.ReadFile(path)
			if err != nil {
				// fmt.Printf("⚠️ [%s:%s] Failed to load telegram template from %s: %v\n", service, lang, path, err)
			} else {
				templates[lang] = string(content)
				// fmt.Printf("📨 [%s:%s] Loaded Telegram template from %s\n", service, lang, path)
			}
		} else {
			// fmt.Printf("⚠️ [%s:%s] No telegram path provided\n", service, lang)
		}
	}
	return templates
}

func loadEmailSubjectPaths(prefix string, service string, langs []string) map[string]string {
	paths := make(map[string]string)
	for _, lang := range langs {
		lang = strings.TrimSpace(strings.ToLower(lang))
		key := fmt.Sprintf("%s_EMAIL_SUBJECT_%s_PATH", prefix, strings.ToUpper(lang))
		val := os.Getenv(key)
		if val != "" {
			paths[lang] = val
			// fmt.Printf("📌 [%s:%s] Registered subject path: %s\n", service, lang, val)
		}
	}
	return paths
}

func loadEmailBodyPaths(prefix string, service string, langs []string) map[string]string {
	paths := make(map[string]string)
	for _, lang := range langs {
		lang = strings.TrimSpace(strings.ToLower(lang))
		key := fmt.Sprintf("%s_EMAIL_BODY_%s_PATH", prefix, strings.ToUpper(lang))
		val := os.Getenv(key)
		if val != "" {
			paths[lang] = val
			// fmt.Printf("📌 [%s:%s] Registered body path: %s\n", service, lang, val)
		}
	}
	return paths
}

func loadTelegramPaths(prefix string, service string, langs []string) map[string]string {
	paths := make(map[string]string)
	for _, lang := range langs {
		lang = strings.TrimSpace(strings.ToLower(lang))
		key := fmt.Sprintf("%s_TG_MSG_%s_PATH", prefix, strings.ToUpper(lang))
		val := os.Getenv(key)
		if val != "" {
			paths[lang] = val
			// fmt.Printf("📌 [%s:%s] Registered Telegram template path: %s\n", service, lang, val)
		}
	}
	return paths
}
