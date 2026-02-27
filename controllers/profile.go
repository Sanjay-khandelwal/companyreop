package controllers

import (
	"net/http"
	"salonpro-backend/config"
	"salonpro-backend/models"
	"salonpro-backend/services"
	"salonpro-backend/utils"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type UpdateProfileInput struct {
	SalonName    string `json:"salonName"`
	SalonAddress string `json:"salonAddress"`
	Phone        string `json:"phone"`
	Email        string `json:"email"`
	// WorkingHours models.JSONB `json:"workingHours"` // or your working hours struct
}

func GetProfile(c *gin.Context) {
	// Get user ID from context
	userIDRaw, exists := c.Get("userId")
	if !exists {
		utils.RespondWithError(c, http.StatusUnauthorized, "User ID not found")
		return
	}
	userUUID, err := uuid.Parse(userIDRaw.(string))
	if err != nil {
		utils.RespondWithError(c, http.StatusInternalServerError, "Invalid user ID format")
		return
	}

	// --- Fetch user ---
	var user models.User
	if err := config.DB.First(&user, "id = ?", userUUID).Error; err != nil {
		utils.RespondWithError(c, http.StatusInternalServerError, "User not found")
		return
	}

	// --- Fetch salon profile using user's SalonID ---
	var salon models.Salon
	if err := config.DB.First(&salon, "id = ?", user.SalonID).Error; err != nil {
		utils.RespondWithError(c, http.StatusInternalServerError, "Salon not found")
		return
	}

	// --- Fetch reminder templates ---
	var reminderTemplates []models.ReminderTemplate
	if err := config.DB.Where("salon_id = ?", salon.ID).Find(&reminderTemplates).Error; err != nil {
		utils.RespondWithError(c, http.StatusInternalServerError, "Failed to fetch reminder templates")
		return
	}

	// Extract messages
	var birthdayMessage, anniversaryMessage string
	for _, tmpl := range reminderTemplates {
		switch tmpl.Type {
		case "birthday":
			birthdayMessage = tmpl.Message
		case "anniversary":
			anniversaryMessage = tmpl.Message
		}
	}

	// --- Return combined response ---
	c.JSON(http.StatusOK, gin.H{
		"salonProfile": gin.H{
			"salonName":    salon.Name,
			"address":      salon.Address,
			"phone":        user.Phone,
			"email":        user.Email,
			"workingHours": salon.WorkingHours,
		},
		"messageTemplates": gin.H{
			"birthday":    birthdayMessage,
			"anniversary": anniversaryMessage,
		},
		"notifications": gin.H{
			"birthdayReminders":     salon.BirthdayReminders,
			"anniversaryReminders":  salon.AnniversaryReminders,
			"whatsAppNotifications": salon.WhatsAppNotifications,
			"smsNotifications":      salon.SMSNotifications,
		},
	})
}

type UpdateSalonProfileInput struct {
	SalonName string `json:"salonName"`
	Address   string `json:"salonAddress"`
	Phone     string `json:"phone"`
	Email     string `json:"email"`
}

func UpdateSalonProfile(c *gin.Context) {
	salonID, exists := c.Get("salonId")
	if !exists {
		utils.RespondWithError(c, http.StatusUnauthorized, "Salon ID not found")
		return
	}
	salonUUID, err := uuid.Parse(salonID.(string))
	if err != nil {
		utils.RespondWithError(c, http.StatusBadRequest, "Invalid salon ID")
		return
	}

	// Get current user ID (assuming you've stored it in the context)
	userIDStr, exists := c.Get("userId")
	if !exists {
		utils.RespondWithError(c, http.StatusUnauthorized, "User ID not found")
		return
	}
	userUUID, err := uuid.Parse(userIDStr.(string))
	if err != nil {
		utils.RespondWithError(c, http.StatusBadRequest, "Invalid user ID")
		return
	}

	// Parse the input
	var input UpdateSalonProfileInput
	if err := c.ShouldBindJSON(&input); err != nil {
		utils.RespondWithError(c, http.StatusBadRequest, "Invalid input: "+err.Error())
		return
	}

	// ✅ Update the salons table
	if err := config.DB.Model(&models.Salon{}).
		Where("id = ?", salonUUID).
		Updates(map[string]interface{}{
			"name":    input.SalonName,
			"address": input.Address,
		}).Error; err != nil {
		utils.RespondWithError(c, http.StatusInternalServerError, "Failed to update salon info")
		return
	}

	// ✅ Check if email is used by another user
	var existingUser models.User
	if err := config.DB.Where("email = ? AND id <> ?", input.Email, userUUID).First(&existingUser).Error; err == nil {
		utils.RespondWithError(c, http.StatusConflict, "Email already in use by another user")
		return
	}

	// ✅ Update only the current user
	if err := config.DB.Model(&models.User{}).
		Where("id = ?", userUUID).
		Updates(map[string]interface{}{
			"phone": input.Phone,
			"email": input.Email,
		}).Error; err != nil {
		utils.RespondWithError(c, http.StatusInternalServerError, "Failed to update user contact info")
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Profile updated successfully"})
}
type UpdateWorkingHoursInput struct {
	WorkingHours models.JSONB `json:"workingHours"`
}

func UpdateWorkingHours(c *gin.Context) {
	salonID, exists := c.Get("salonId")
	if !exists {
		utils.RespondWithError(c, http.StatusUnauthorized, "Salon ID not found")
		return
	}
	salonUUID, err := uuid.Parse(salonID.(string))
	if err != nil {
		utils.RespondWithError(c, http.StatusBadRequest, "Invalid salon ID")
		return
	}

	var input UpdateWorkingHoursInput
	if err := c.ShouldBindJSON(&input); err != nil {
		utils.RespondWithError(c, http.StatusBadRequest, "Invalid input: "+err.Error())
		return
	}

	if err := config.DB.Model(&models.Salon{}).
		Where("id = ?", salonUUID).
		Update("working_hours", input.WorkingHours).Error; err != nil {
		utils.RespondWithError(c, http.StatusInternalServerError, "Failed to update working hours")
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Working hours updated successfully"})
}

type UpdateTemplatesInput struct {
	BirthdayMessage    string `json:"birthday" form:"birthday" binding:"omitempty"`
	AnniversaryMessage string `json:"anniversary" form:"anniversary" binding:"omitempty"`
}

func UpdateReminderTemplates(c *gin.Context) {
	salonID, exists := c.Get("salonId")
	if !exists {
		utils.RespondWithError(c, http.StatusUnauthorized, "Salon ID not found")
		return
	}
	salonUUID, err := uuid.Parse(salonID.(string))
	if err != nil {
		utils.RespondWithError(c, http.StatusBadRequest, "Invalid salon ID")
		return
	}

	var input UpdateTemplatesInput
	if err := c.ShouldBindJSON(&input); err != nil {
		utils.RespondWithError(c, http.StatusBadRequest, "Invalid input: "+err.Error())
		return
	}

	updates := []struct {
		Type    string
		Message string
	}{
		{"birthday", input.BirthdayMessage},
		{"anniversary", input.AnniversaryMessage},
	}

	for _, u := range updates {
		if err := config.DB.Model(&models.ReminderTemplate{}).
			Where("salon_id = ? AND type = ?", salonUUID, u.Type).
			Update("message", u.Message).Error; err != nil {
			utils.RespondWithError(c, http.StatusInternalServerError, "Failed to update "+u.Type+" template")
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"message": "Templates updated successfully"})
}

type UpdateNotificationsInput struct {
	BirthdayReminders     bool `json:"birthdayReminders"`
	AnniversaryReminders  bool `json:"anniversaryReminders"`
	WhatsAppNotifications bool `json:"whatsAppNotifications"`
	SMSNotifications      bool `json:"smsNotifications"`
}

func UpdateNotifications(c *gin.Context) {
	salonID, exists := c.Get("salonId")
	if !exists {
		utils.RespondWithError(c, http.StatusUnauthorized, "Salon ID not found")
		return
	}
	salonUUID, err := uuid.Parse(salonID.(string))
	if err != nil {
		utils.RespondWithError(c, http.StatusBadRequest, "Invalid salon ID")
		return
	}

	var input UpdateNotificationsInput
	if err := c.ShouldBindJSON(&input); err != nil {
		utils.RespondWithError(c, http.StatusBadRequest, "Invalid input: "+err.Error())
		return
	}

	if err := config.DB.Model(&models.Salon{}).
		Where("id = ?", salonUUID).
		Updates(map[string]interface{}{
			"birthday_reminders":      input.BirthdayReminders,
			"anniversary_reminders":   input.AnniversaryReminders,
			"whats_app_notifications": input.WhatsAppNotifications,
			"sms_notifications":       input.SMSNotifications,
		}).Error; err != nil {
		utils.RespondWithError(c, http.StatusInternalServerError, "Failed to update notifications")
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Notification settings updated successfully"})
}

// TestNotificationInput is the body for sending a test SMS or WhatsApp message.
type TestNotificationInput struct {
	Phone   string `json:"phone" binding:"required"`   // E.164 format, e.g. +919799570493
	Message string `json:"message"`                    // Optional: if empty, uses salon's reminder template body (with [CustomerName] → "Test Customer")
	Channel string `json:"channel" binding:"required"` // "sms" or "whatsapp"
}

// SendTestNotification sends a single test SMS or WhatsApp message (for testing Twilio).
// If "message" is omitted or empty, uses the current implementation body from the salon's reminder template (same as real reminders).
// POST /auth/profile/test-notification with body: { "phone": "+919799570493", "channel": "sms" } or include "message" to override.
func SendTestNotification(c *gin.Context) {
	var input TestNotificationInput
	if err := c.ShouldBindJSON(&input); err != nil {
		utils.RespondWithError(c, http.StatusBadRequest, "Invalid input: phone and channel are required")
		return
	}
	channel := strings.ToLower(strings.TrimSpace(input.Channel))
	if channel != "sms" && channel != "whatsapp" {
		utils.RespondWithError(c, http.StatusBadRequest, "channel must be 'sms' or 'whatsapp'")
		return
	}
	phone := strings.TrimSpace(input.Phone)
	if phone == "" {
		utils.RespondWithError(c, http.StatusBadRequest, "phone is required (E.164 format, e.g. +919799570493)")
		return
	}

	body := strings.TrimSpace(input.Message)
	if body == "" {
		salonID, exists := c.Get("salonId")
		if !exists {
			utils.RespondWithError(c, http.StatusUnauthorized, "Salon ID not found")
			return
		}
		salonUUID, err := uuid.Parse(salonID.(string))
		if err != nil {
			utils.RespondWithError(c, http.StatusBadRequest, "Invalid salon ID")
			return
		}
		var templates []models.ReminderTemplate
		if err := config.DB.Where("salon_id = ? AND is_active = true", salonUUID).Find(&templates).Error; err != nil {
			utils.RespondWithError(c, http.StatusInternalServerError, "Failed to fetch reminder templates")
			return
		}
		// Use first available template (birthday preferred), same as current reminder implementation
		for _, t := range templates {
			if t.Type == "birthday" && t.Message != "" {
				body = strings.ReplaceAll(t.Message, "[CustomerName]", "Test Customer")
				break
			}
		}
		if body == "" {
			for _, t := range templates {
				if t.Type == "anniversary" && t.Message != "" {
					body = strings.ReplaceAll(t.Message, "[CustomerName]", "Test Customer")
					break
				}
			}
		}
		if body == "" {
			body = "Test reminder from SalonPro – [CustomerName]"
		}
	}

	svc := services.NewReminderService(config.DB)
	if err := svc.SendTestMessage(phone, body, channel); err != nil {
		utils.RespondWithError(c, http.StatusInternalServerError, "Failed to send test notification: "+err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"message": "Test " + channel + " sent successfully",
		"channel": channel,
		"phone":   phone,
		"body":    body,
	})
}
