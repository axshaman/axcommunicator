package utils

import (
	"fmt"
	"os"      // ✅ добавлено: нужно для os.ReadFile
	"strings"
)

// Contains checks if a string is in a slice
func Contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// FillTemplate replaces placeholders in a template with values from a map
func FillTemplate(template string, data map[string]interface{}) string {
	result := template
	for key, value := range data {
		placeholder := "{" + key + "}"
		strValue, ok := value.(string)
		if !ok {
			strValue = fmt.Sprintf("%v", value)
		}
		result = strings.ReplaceAll(result, placeholder, strValue)
	}
	return result
}

// LoadTemplateOrDefault reads a template file or uses the fallback if missing
func LoadTemplateOrDefault(path, fallback string, data map[string]interface{}) string {
	content, err := os.ReadFile(path)
	if err != nil {
		return FillTemplate(fallback, data)
	}
	return FillTemplate(string(content), data)
}
