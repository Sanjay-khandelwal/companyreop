package models

import (
	"time"

	"github.com/google/uuid"
)

type Invoice struct {
	ID              uuid.UUID `gorm:"type:uuid;primary_key;default:uuid_generate_v4()"`
	SalonID         uuid.UUID `gorm:"type:uuid;index;not null"`
	CreatedByUserID uuid.UUID `gorm:"type:uuid;index;not null"`

	InvoiceNumber string    `gorm:"uniqueIndex;not null"`
	CustomerID    uuid.UUID `gorm:"type:uuid;index;not null"`
	InvoiceDate   time.Time `gorm:"default:CURRENT_TIMESTAMP"`

	Subtotal float64 `gorm:"type:decimal(10,2);not null"`
	Discount float64 `gorm:"type:decimal(10,2);default:0.0"`
	Tax      float64 `gorm:"type:decimal(10,2);default:0.0"`
	Total    float64 `gorm:"type:decimal(10,2);not null"`

	PaymentStatus string  `gorm:"type:payment_status;default:'unpaid'"`
	PaidAmount    float64 `gorm:"type:decimal(10,2);default:0.0"`
	PaymentMethod string
	Notes         string

	Items []InvoiceItem `gorm:"foreignKey:InvoiceID"`
}

type InvoiceItem struct {
	ID          uuid.UUID `gorm:"type:uuid;primary_key;default:uuid_generate_v4()"`
	InvoiceID   uuid.UUID `gorm:"type:uuid;index;not null"`
	ServiceID   uuid.UUID `gorm:"type:uuid;index;not null"`
	ServiceName string    `gorm:"not null"`
	Quantity    int       `gorm:"default:1"`
	UnitPrice   float64   `gorm:"type:decimal(10,2);not null"`
	TotalPrice  float64   `gorm:"type:decimal(10,2);not null"`
}
