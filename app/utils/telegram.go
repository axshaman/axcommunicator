package utils

import (
	"axcommutator/app/config"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	telegramAPITimeout = 15 * time.Second
	telegramAPIBaseURL = "https://api.telegram.org/bot%s/sendMessage"
	maxMessageLength   = 4096
)

var (
	ErrTelegramNotConfigured = errors.New("telegram not configured for this service")
	ErrInvalidTemplate       = errors.New("invalid template format")
	ErrMessageTooLong        = errors.New("message exceeds maximum length")
	ErrAPIRequestFailed      = errors.New("telegram API request failed")
)

type TelegramResponse struct {
	OK          bool   `json:"ok"`
	Description string `json:"description"`
	ErrorCode   int    `json:"error_code"`
}

// SendTelegramNotification sends a localized notification to Telegram
func SendTelegramNotification(service config.ServiceConfig, lang string, data map[string]interface{}) error {
	if !service.Telegram.Configured() {
		return ErrTelegramNotConfigured
	}

	template, err := getLocalizedTemplate(service, lang)
	if err != nil {
		return fmt.Errorf("template error: %w", err)
	}

	message, err := renderTemplate(template, data)
	if err != nil {
		return fmt.Errorf("template rendering failed: %w", err)
	}

	message = EscapeMarkdownV2(message)

	if len(message) > maxMessageLength {
		return fmt.Errorf("%w: %d > %d", ErrMessageTooLong, len(message), maxMessageLength)
	}

	response, err := sendTelegramRequest(service.Telegram.BotToken, service.Telegram.ChatID, message)
	if err != nil {
		return fmt.Errorf("send failed: %w", err)
	}

	if !response.OK {
		return fmt.Errorf("telegram API error: %s (code %d)", response.Description, response.ErrorCode)
	}

	return nil
}

func getLocalizedTemplate(service config.ServiceConfig, lang string) (string, error) {
	if _, exists := service.TelegramTemplates[lang]; !exists {
		if len(service.SupportedLangs) == 0 {
			return "", ErrInvalidTemplate
		}
		lang = service.SupportedLangs[0]
	}
	// 🔥 Load from disk if TelegramTemplatePaths set
	if path, ok := service.TelegramTemplatePaths[lang]; ok && path != "" {
		content, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("failed to load Telegram template from %s: %w", path, err)
		}
		return string(content), nil
	}

	// Fallback to in-memory
	template, ok := service.TelegramTemplates[lang]
	if !ok || template == "" {
		return "", ErrInvalidTemplate
	}
	return template, nil
}

func renderTemplate(template string, data map[string]interface{}) (string, error) {
	result := template
	for key, value := range data {
		placeholder := "{" + key + "}"
		strValue := fmt.Sprintf("%v", value)
		result = strings.ReplaceAll(result, placeholder, strValue)
	}
	return result, nil
}

// EscapeMarkdownV2 escapes all required characters for Telegram MarkdownV2 format
func EscapeMarkdownV2(text string) string {
	specials := []string{
		"_", "*", "[", "]", "(", ")", "~", "`", ">", "#",
		"+", "-", "=", "|", "{", "}", ".", "!",
	}
	for _, s := range specials {
		text = strings.ReplaceAll(text, s, "\\"+s)
	}
	return text
}

func sendTelegramRequest(botToken, chatID, message string) (*TelegramResponse, error) {
	payload := map[string]interface{}{
		"chat_id":                  chatID,
		"text":                     message,
		"parse_mode":               "MarkdownV2",
		"disable_web_page_preview": true,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("payload marshal failed: %w", err)
	}

	client := &http.Client{Timeout: telegramAPITimeout}
	apiURL := fmt.Sprintf(telegramAPIBaseURL, botToken)

	resp, err := client.Post(apiURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrAPIRequestFailed, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("response read failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, string(body))
	}

	var response TelegramResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("response parse failed: %w", err)
	}

	return &response, nil
}

// SendTelegramDocument sends a file (PDF, DOC, etc.) to Telegram with caption
func SendTelegramDocument(botToken, chatID, filePath, caption string) error {
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendDocument", botToken)

	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("cannot open file: %w", err)
	}
	defer file.Close()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	part, err := writer.CreateFormFile("document", filepath.Base(filePath))
	if err != nil {
		return fmt.Errorf("create form file failed: %w", err)
	}
	if _, err = io.Copy(part, file); err != nil {
		return fmt.Errorf("copy file failed: %w", err)
	}

	_ = writer.WriteField("chat_id", chatID)
	_ = writer.WriteField("caption", EscapeMarkdownV2(caption))
	_ = writer.WriteField("parse_mode", "MarkdownV2")
	writer.Close()

	req, err := http.NewRequest("POST", apiURL, body)
	if err != nil {
		return fmt.Errorf("request build failed: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	client := &http.Client{Timeout: telegramAPITimeout}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("send failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("telegram error: %s", respBody)
	}

	var tgResp TelegramResponse
	if err := json.Unmarshal(respBody, &tgResp); err != nil {
		return fmt.Errorf("response parse failed: %w", err)
	}
	if !tgResp.OK {
		return fmt.Errorf("telegram error: %s (code %d)", tgResp.Description, tgResp.ErrorCode)
	}

	return nil
}
