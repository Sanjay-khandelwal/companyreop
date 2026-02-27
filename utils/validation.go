// utils/validation.go
package utils

import (
	"regexp"
	"strings"
)

// ValidatePhone checks if a phone number is in a valid international format
func ValidatePhone(phone string) bool {
	// Clean the phone number
	cleaned := strings.ReplaceAll(phone, " ", "")
	cleaned = strings.ReplaceAll(cleaned, "-", "")
	cleaned = strings.ReplaceAll(cleaned, "(", "")
	cleaned = strings.ReplaceAll(cleaned, ")", "")
	
	// Regular expression for international phone numbers
	// Allows + prefix followed by 7-15 digits
	regex := `^\+?[1-9]\d{1,14}$`
	match, _ := regexp.MatchString(regex, cleaned)
	return match
}