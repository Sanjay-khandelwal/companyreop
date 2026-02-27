// controllers/invoice.go
package controllers

import (
	"errors"
	"net/http"
	"time"

	"salonpro-backend/config"
	"salonpro-backend/models"
	"salonpro-backend/utils"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// InvoiceItemInput defines the structure for an invoice item
type InvoiceItemInput struct {
	ServiceID uuid.UUID `json:"serviceId" binding:"required"`
	Quantity  int       `json:"quantity" binding:"min=1"`
}

// CreateInvoiceInput defines the expected JSON structure for creating an invoice
type CreateInvoiceInput struct {
	CustomerID    uuid.UUID          `json:"customerId" binding:"required"`
	InvoiceDate   *time.Time         `json:"invoiceDate"`
	Items         []InvoiceItemInput `json:"items" binding:"required,min=1"`
	Discount      float64            `json:"discount" binding:"min=0"`
	Tax           float64            `json:"tax" binding:"min=0"`
	PaymentStatus string             `json:"paymentStatus" binding:"oneof=paid unpaid partial"`
	PaidAmount    float64            `json:"paidAmount" binding:"min=0"`
	PaymentMethod string             `json:"paymentMethod"`
	Notes         string             `json:"notes"`
}

// UpdateInvoiceInput defines the expected JSON structure for updating an invoice
type UpdateInvoiceInput struct {
	CustomerID    *uuid.UUID          `json:"customerId"`
	InvoiceDate   *time.Time          `json:"invoiceDate"`
	Items         *[]InvoiceItemInput `json:"items"`
	Discount      *float64            `json:"discount"`
	Tax           *float64            `json:"tax"`
	PaymentStatus *string             `json:"paymentStatus" binding:"omitempty,oneof=paid unpaid partial"`
	PaidAmount    *float64            `json:"paidAmount" binding:"omitempty,min=0"`
	PaymentMethod *string             `json:"paymentMethod"`
	Notes         *string             `json:"notes"`
}

// CreateInvoice creates a new invoice for the salon
func CreateInvoice(c *gin.Context) {
	salonID, exists := c.Get("salonId")
	if !exists {
		utils.RespondWithError(c, http.StatusUnauthorized, "Salon ID not found in context")
		return
	}

	userID, exists := c.Get("userId")
	if !exists {
		utils.RespondWithError(c, http.StatusUnauthorized, "User ID not found in context")
		return
	}

	salonUUID, err := uuid.Parse(salonID.(string))
	if err != nil {
		utils.RespondWithError(c, http.StatusInternalServerError, "Invalid salon ID format")
		return
	}

	var input CreateInvoiceInput
	if err := c.ShouldBindJSON(&input); err != nil {
		utils.RespondWithError(c, http.StatusBadRequest, "Invalid input: "+err.Error())
		return
	}

	// Validate customer exists in the same salon
	var customer models.Customer
	if err := config.DB.Where("salon_id = ? AND id = ?", salonUUID, input.CustomerID).
		First(&customer).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			utils.RespondWithError(c, http.StatusBadRequest, "Customer not found")
		} else {
			utils.RespondWithError(c, http.StatusInternalServerError, "Database error")
		}
		return
	}

	// Validate and calculate invoice items
	var subtotal float64 = 0
	var invoiceItems []models.InvoiceItem

	for _, item := range input.Items {
		// Validate service exists and belongs to the same salon
		var service models.Service
		if err := config.DB.Where("salon_id = ? AND id = ?", salonUUID, item.ServiceID).
			First(&service).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				utils.RespondWithError(c, http.StatusBadRequest, "Service not found: "+item.ServiceID.String())
			} else {
				utils.RespondWithError(c, http.StatusInternalServerError, "Database error")
			}
			return
		}

		// Calculate item total
		itemTotal := service.Price * float64(item.Quantity)
		subtotal += itemTotal

		invoiceItems = append(invoiceItems, models.InvoiceItem{
			ID:          uuid.New(),
			ServiceID:   service.ID,
			ServiceName: service.Name,
			Quantity:    item.Quantity,
			UnitPrice:   service.Price,
			TotalPrice:  itemTotal,
		})
	}

	// Calculate total
	total := subtotal - input.Discount + (subtotal * input.Tax / 100)

	// Set default invoice date to now if not provided
	invoiceDate := time.Now()
	if input.InvoiceDate != nil {
		invoiceDate = *input.InvoiceDate
	}

	// Create new invoice
	invoice := models.Invoice{
		ID:              uuid.New(),
		CreatedByUserID: uuid.Must(uuid.Parse(userID.(string))),
		SalonID:         salonUUID,
		CustomerID:      input.CustomerID,
		InvoiceDate:     invoiceDate,
		Subtotal:        subtotal,
		Discount:        input.Discount,
		Tax:             input.Tax,
		Total:           total,
		PaymentStatus:   input.PaymentStatus,
		PaidAmount:      input.PaidAmount,
		PaymentMethod:   input.PaymentMethod,
		Notes:           input.Notes,
		Items:           invoiceItems,
	}

	// Generate invoice number (you might want a better way)
	invoice.InvoiceNumber = "INV-" + time.Now().Format("20060102") + "-" + utils.GenerateRandomString(6)

	// Start transaction
	tx := config.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Save invoice
	if err := tx.Create(&invoice).Error; err != nil {
		tx.Rollback()
		utils.RespondWithError(c, http.StatusInternalServerError, "Failed to create invoice")
		return
	}

	// Update customer stats
	if err := tx.Model(&models.Customer{}).Where("id = ?", input.CustomerID).
		Updates(map[string]interface{}{
			"total_visits": gorm.Expr("total_visits + ?", 1),
			"total_spent":  gorm.Expr("total_spent + ?", total),
			"last_visit":   invoiceDate,
		}).Error; err != nil {
		tx.Rollback()
		utils.RespondWithError(c, http.StatusInternalServerError, "Failed to update customer stats")
		return
	}

	tx.Commit()

	c.JSON(http.StatusCreated, invoice)
}

// GetInvoices retrieves all invoices for the salon
func GetInvoices(c *gin.Context) {
	salonID, exists := c.Get("salonId")
	if !exists {
		utils.RespondWithError(c, http.StatusUnauthorized, "Salon ID not found in context")
		return
	}

	salonUUID, err := uuid.Parse(salonID.(string))
	if err != nil {
		utils.RespondWithError(c, http.StatusInternalServerError, "Invalid salon ID format")
		return
	}

	var invoices []models.Invoice
	if err := config.DB.Preload("Items").
		Where("salon_id = ?", salonUUID).
		Find(&invoices).Error; err != nil {
		utils.RespondWithError(c, http.StatusInternalServerError, "Failed to retrieve invoices")
		return
	}

	c.JSON(http.StatusOK, invoices)
}

// GetInvoice retrieves a specific invoice by ID
func GetInvoice(c *gin.Context) {
	salonID, exists := c.Get("salonId")
	if !exists {
		utils.RespondWithError(c, http.StatusUnauthorized, "Salon ID not found in context")
		return
	}

	salonUUID, err := uuid.Parse(salonID.(string))
	if err != nil {
		utils.RespondWithError(c, http.StatusInternalServerError, "Invalid salon ID format")
		return
	}

	invoiceID := c.Param("id")
	invoiceUUID, err := uuid.Parse(invoiceID)
	if err != nil {
		utils.RespondWithError(c, http.StatusBadRequest, "Invalid invoice ID format")
		return
	}

	var invoice models.Invoice
	if err := config.DB.Preload("Items").
		Where("salon_id = ? AND id = ?", salonUUID, invoiceUUID).
		First(&invoice).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			utils.RespondWithError(c, http.StatusNotFound, "Invoice not found")
		} else {
			utils.RespondWithError(c, http.StatusInternalServerError, "Database error")
		}
		return
	}

	c.JSON(http.StatusOK, invoice)
}

// UpdateInvoice updates an existing invoice
func UpdateInvoice(c *gin.Context) {
	salonID, exists := c.Get("salonId")
	if !exists {
		utils.RespondWithError(c, http.StatusUnauthorized, "Salon ID not found in context")
		return
	}

	salonUUID, err := uuid.Parse(salonID.(string))
	if err != nil {
		utils.RespondWithError(c, http.StatusInternalServerError, "Invalid salon ID format")
		return
	}

	invoiceID := c.Param("id")
	invoiceUUID, err := uuid.Parse(invoiceID)
	if err != nil {
		utils.RespondWithError(c, http.StatusBadRequest, "Invalid invoice ID format")
		return
	}

	var input UpdateInvoiceInput
	if err := c.ShouldBindJSON(&input); err != nil {
		utils.RespondWithError(c, http.StatusBadRequest, "Invalid input: "+err.Error())
		return
	}

	// Start transaction
	tx := config.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Retrieve existing invoice
	var invoice models.Invoice
	if err := tx.Preload("Items").
		Where("salon_id = ? AND id = ?", salonUUID, invoiceUUID).
		First(&invoice).Error; err != nil {
		tx.Rollback()
		if errors.Is(err, gorm.ErrRecordNotFound) {
			utils.RespondWithError(c, http.StatusNotFound, "Invoice not found")
		} else {
			utils.RespondWithError(c, http.StatusInternalServerError, "Database error")
		}
		return
	}

	// Update fields if provided
	if input.CustomerID != nil {
		// Validate customer exists in the same salon
		var customer models.Customer
		if err := tx.Where("salon_id = ? AND id = ?", salonUUID, *input.CustomerID).
			First(&customer).Error; err != nil {
			tx.Rollback()
			if errors.Is(err, gorm.ErrRecordNotFound) {
				utils.RespondWithError(c, http.StatusBadRequest, "Customer not found")
			} else {
				utils.RespondWithError(c, http.StatusInternalServerError, "Database error")
			}
			return
		}
		invoice.CustomerID = *input.CustomerID
	}

	if input.InvoiceDate != nil {
		invoice.InvoiceDate = *input.InvoiceDate
	}

	// If items are being updated, recalculate the invoice
	if input.Items != nil {
		var subtotal float64 = 0
		var newInvoiceItems []models.InvoiceItem

		// Delete existing items
		if err := tx.Where("invoice_id = ?", invoice.ID).Delete(&models.InvoiceItem{}).Error; err != nil {
			tx.Rollback()
			utils.RespondWithError(c, http.StatusInternalServerError, "Failed to clear existing items")
			return
		}

		// Create new items
		for _, item := range *input.Items {
			// Validate service exists and belongs to the same salon
			var service models.Service
			if err := tx.Where("salon_id = ? AND id = ?", salonUUID, item.ServiceID).
				First(&service).Error; err != nil {
				tx.Rollback()
				if errors.Is(err, gorm.ErrRecordNotFound) {
					utils.RespondWithError(c, http.StatusBadRequest, "Service not found: "+item.ServiceID.String())
				} else {
					utils.RespondWithError(c, http.StatusInternalServerError, "Database error")
				}
				return
			}

			// Calculate item total
			itemTotal := service.Price * float64(item.Quantity)
			subtotal += itemTotal

			newInvoiceItems = append(newInvoiceItems, models.InvoiceItem{
				InvoiceID:   invoice.ID,
				ServiceID:   service.ID,
				ServiceName: service.Name,
				Quantity:    item.Quantity,
				UnitPrice:   service.Price,
				TotalPrice:  itemTotal,
			})
		}

		invoice.Items = newInvoiceItems
		invoice.Subtotal = subtotal
	}

	if input.Discount != nil {
		invoice.Discount = *input.Discount
	}

	if input.Tax != nil {
		invoice.Tax = *input.Tax
	}

	// Recalculate total if needed
	if input.Items != nil || input.Discount != nil || input.Tax != nil {
		invoice.Total = invoice.Subtotal - invoice.Discount + (invoice.Subtotal * invoice.Tax / 100)
	}

	if input.PaymentStatus != nil {
		invoice.PaymentStatus = *input.PaymentStatus
	}

	if input.PaidAmount != nil {
		invoice.PaidAmount = *input.PaidAmount
	}

	if input.PaymentMethod != nil {
		invoice.PaymentMethod = *input.PaymentMethod
	}

	if input.Notes != nil {
		invoice.Notes = *input.Notes
	}

	// Save updated invoice
	if err := tx.Save(&invoice).Error; err != nil {
		tx.Rollback()
		utils.RespondWithError(c, http.StatusInternalServerError, "Failed to update invoice")
		return
	}

	tx.Commit()

	c.JSON(http.StatusOK, invoice)
}

// DeleteInvoice soft deletes an invoice
func DeleteInvoice(c *gin.Context) {
	salonID, exists := c.Get("salonId")
	if !exists {
		utils.RespondWithError(c, http.StatusUnauthorized, "Salon ID not found in context")
		return
	}

	salonUUID, err := uuid.Parse(salonID.(string))
	if err != nil {
		utils.RespondWithError(c, http.StatusInternalServerError, "Invalid salon ID format")
		return
	}

	invoiceID := c.Param("id")
	invoiceUUID, err := uuid.Parse(invoiceID)
	if err != nil {
		utils.RespondWithError(c, http.StatusBadRequest, "Invalid invoice ID format")
		return
	}

	// Start transaction
	tx := config.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Retrieve invoice to get customer and total
	var invoice models.Invoice
	if err := tx.Where("salon_id = ? AND id = ?", salonUUID, invoiceUUID).
		First(&invoice).Error; err != nil {
		tx.Rollback()
		if errors.Is(err, gorm.ErrRecordNotFound) {
			utils.RespondWithError(c, http.StatusNotFound, "Invoice not found")
		} else {
			utils.RespondWithError(c, http.StatusInternalServerError, "Database error")
		}
		return
	}

	// Delete invoice items
	if err := tx.Where("invoice_id = ?", invoice.ID).Delete(&models.InvoiceItem{}).Error; err != nil {
		tx.Rollback()
		utils.RespondWithError(c, http.StatusInternalServerError, "Failed to delete invoice items")
		return
	}

	// Delete invoice
	if err := tx.Delete(&invoice).Error; err != nil {
		tx.Rollback()
		utils.RespondWithError(c, http.StatusInternalServerError, "Failed to delete invoice")
		return
	}

	// Update customer stats (decrement)
	if err := tx.Model(&models.Customer{}).Where("id = ?", invoice.CustomerID).
		Updates(map[string]interface{}{
			"total_visits": gorm.Expr("total_visits - ?", 1),
			"total_spent":  gorm.Expr("total_spent - ?", invoice.Total),
		}).Error; err != nil {
		tx.Rollback()
		utils.RespondWithError(c, http.StatusInternalServerError, "Failed to update customer stats")
		return
	}

	tx.Commit()

	c.JSON(http.StatusOK, gin.H{"message": "Invoice deleted successfully"})
}
