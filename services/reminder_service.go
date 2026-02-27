// services/reminder_service.go
package services

import (
	"fmt"
	"log"
	"os"
	"salonpro-backend/models"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/robfig/cron/v3"
	"github.com/twilio/twilio-go"
	twilioApi "github.com/twilio/twilio-go/rest/api/v2010"
	"gorm.io/gorm"
)

type ReminderService struct {
	db     *gorm.DB
	client *twilio.RestClient
}

func NewReminderService(db *gorm.DB) *ReminderService {
	accountSid := strings.TrimSpace(os.Getenv("TWILIO_ACCOUNT_SID"))
	authToken := strings.TrimSpace(os.Getenv("TWILIO_AUTH_TOKEN"))

	var client *twilio.RestClient
	if accountSid != "" && authToken != "" {
		client = twilio.NewRestClientWithParams(twilio.ClientParams{
			Username: accountSid,
			Password: authToken,
		})
		log.Println("Twilio client initialized; notifications will be sent when scheduler runs.")
	} else {
		log.Println("Twilio not configured (TWILIO_ACCOUNT_SID or TWILIO_AUTH_TOKEN missing). Reminder notifications disabled.")
	}

	return &ReminderService{
		db:     db,
		client: client,
	}
}

func (s *ReminderService) StartScheduler() {
	if s.client == nil {
		log.Println("Reminder scheduler not started: Twilio client is not configured.")
		return
	}
	c := cron.New()
	_, _ = c.AddFunc("0 9 * * *", s.SendDailyReminders) // Every day at 9 AM
	c.Start()
	s.SendDailyReminders() // Run once on server startup
	log.Println("Reminder scheduler started (runs daily at 9 AM and once on startup)")
}

func (s *ReminderService) SendDailyReminders() {
	if s.client == nil {
		return
	}
	log.Println("Starting daily reminder processing...")

	var users []models.User
	if err := s.db.Find(&users, "is_active = ?", true).Error; err != nil {
		log.Printf("Failed to fetch active users: %v", err)
		return
	}

	// Process each salon once (by SalonID)
	seen := make(map[uuid.UUID]bool)
	for _, u := range users {
		if seen[u.SalonID] {
			continue
		}
		seen[u.SalonID] = true
		s.ProcessSalonReminders(u.SalonID)
	}

	log.Println("Daily reminder processing completed")
}

func (s *ReminderService) ProcessSalonReminders(salonID uuid.UUID) {
	var salon models.Salon
	if err := s.db.First(&salon, "id = ?", salonID).Error; err != nil {
		log.Printf("Salon %s: not found: %v", salonID, err)
		return
	}
	// Only send if salon has at least one notification channel enabled
	if !salon.WhatsAppNotifications && !salon.SMSNotifications {
		log.Printf("Salon %s: notifications skipped (enable WhatsApp or SMS in profile)", salonID)
		return
	}

	// Birthdays: only if salon has birthday reminders on
	if salon.BirthdayReminders {
		birthdayCustomers, err := s.getUpcomingCustomers(salonID, "birthday")
		if err != nil {
			log.Printf("Salon %s: Failed to get birthday customers: %v", salonID, err)
		} else {
			s.sendReminders(salonID, birthdayCustomers, "birthday", &salon)
		}
	}

	// Anniversaries: only if salon has anniversary reminders on
	if salon.AnniversaryReminders {
		anniversaryCustomers, err := s.getUpcomingCustomers(salonID, "anniversary")
		if err != nil {
			log.Printf("Salon %s: Failed to get anniversary customers: %v", salonID, err)
		} else {
			s.sendReminders(salonID, anniversaryCustomers, "anniversary", &salon)
		}
	}
}

func (s *ReminderService) getUpcomingCustomers(salonID uuid.UUID, eventType string) ([]models.Customer, error) {
	now := time.Now()

	var customers []models.Customer
	var field string
	switch eventType {
	case "birthday":
		field = "birthday"
	case "anniversary":
		field = "anniversary"
	default:
		return nil, fmt.Errorf("invalid event type: %s", eventType)
	}

	// Build (month, day) pairs for today through today+7 (next 7 days inclusive)
	type monthDay struct{ M, D int }
	var pairs []monthDay
	for d := 0; d <= 7; d++ {
		t := now.AddDate(0, 0, d)
		pairs = append(pairs, monthDay{int(t.Month()), t.Day()})
	}
	// Build IN clause: (EXTRACT(MONTH FROM field), EXTRACT(DAY FROM field)) IN ((1,25),(1,26),...)
	var placeholders []string
	var args []interface{}
	args = append(args, salonID)
	for _, p := range pairs {
		placeholders = append(placeholders, "(?, ?)")
		args = append(args, p.M, p.D)
	}
	inClause := ""
	for i, ph := range placeholders {
		if i > 0 {
			inClause += ", "
		}
		inClause += ph
	}
	query := fmt.Sprintf(`
		SELECT * FROM customers
		WHERE salon_id = ?
		AND is_active = true
		AND %s IS NOT NULL
		AND (EXTRACT(MONTH FROM %s), EXTRACT(DAY FROM %s)) IN (%s)
	`, field, field, field, inClause)

	err := s.db.Raw(query, args...).Scan(&customers).Error
	return customers, err
}

func (s *ReminderService) sendReminders(salonID uuid.UUID, customers []models.Customer, eventType string, salon *models.Salon) {
	var template models.ReminderTemplate
	if err := s.db.Where("salon_id = ? AND type = ? AND is_active = true", salonID, eventType).
		First(&template).Error; err != nil {
		log.Printf("Salon %s: No active template for %s: %v", salonID, eventType, err)
		return
	}

	fromSMS := os.Getenv("TWILIO_PHONE_NUMBER")
	fromWhatsApp := strings.TrimPrefix(strings.TrimSpace(os.Getenv("TWILIO_WHATSAPP_NUMBER")), "whatsapp:")

	for _, customer := range customers {
		if strings.TrimSpace(customer.Phone) == "" {
			continue
		}
		message := strings.ReplaceAll(template.Message, "[CustomerName]", customer.Name)

		channel := "sms"
		to := customer.Phone
		useWhatsApp := salon.WhatsAppNotifications && strings.HasPrefix(customer.Phone, "+") && fromWhatsApp != ""
		useSMS := salon.SMSNotifications && fromSMS != ""

		if useWhatsApp {
			to = "whatsapp:" + customer.Phone
			channel = "whatsapp"
		} else if !useSMS {
			continue // No channel available
		}

		params := &twilioApi.CreateMessageParams{}
		params.SetTo(to)
		params.SetBody(message)
		if channel == "whatsapp" {
			params.SetFrom("whatsapp:" + fromWhatsApp)
		} else {
			params.SetFrom(fromSMS)
		}

		resp, err := s.client.Api.CreateMessage(params)
		if err != nil {
			log.Printf("Failed to send %s reminder to %s: %v", eventType, customer.Phone, err)
		} else if resp.Sid != nil {
			log.Printf("Reminder sent to %s, SID: %s", customer.Phone, *resp.Sid)
		}
		_ = resp
	}
}

// SendTestMessage sends a single SMS or WhatsApp message (for testing).
// channel must be "sms" or "whatsapp". Phone should be E.164 (e.g. +919799570493).
func (s *ReminderService) SendTestMessage(phone, body, channel string) error {
	if s.client == nil {
		return fmt.Errorf("Twilio not configured; set TWILIO_ACCOUNT_SID and TWILIO_AUTH_TOKEN")
	}
	fromSMS := os.Getenv("TWILIO_PHONE_NUMBER")
	fromWhatsApp := strings.TrimPrefix(strings.TrimSpace(os.Getenv("TWILIO_WHATSAPP_NUMBER")), "whatsapp:")

	to := phone
	var from string
	switch channel {
	case "whatsapp":
		if fromWhatsApp == "" {
			return fmt.Errorf("TWILIO_WHATSAPP_NUMBER not set")
		}
		to = "whatsapp:" + phone
		from = "whatsapp:" + fromWhatsApp
	case "sms":
		if fromSMS == "" {
			return fmt.Errorf("TWILIO_PHONE_NUMBER not set")
		}
		from = fromSMS
	default:
		return fmt.Errorf("channel must be sms or whatsapp, got %q", channel)
	}

	params := &twilioApi.CreateMessageParams{}
	params.SetTo(to)
	params.SetFrom(from)
	params.SetBody(body)
	_, err := s.client.Api.CreateMessage(params)
	return err
}
