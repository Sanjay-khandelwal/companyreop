package config

import (
	"os"
	"strings"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var DB *gorm.DB

func ConnectDB() {
	dsn := strings.TrimSpace(os.Getenv("DB_URL"))
	if dsn == "" {
		panic("DB_URL environment variable is not set")
	}
	// Must be a postgres URL; if it starts with "psql" or has quotes, parsing will fail
	if !strings.HasPrefix(dsn, "postgres://") && !strings.HasPrefix(dsn, "postgresql://") {
		panic("DB_URL must be a postgres URL starting with postgres:// or postgresql:// (no 'psql' command or extra quotes)")
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		panic("Failed to connect database: " + err.Error())
	}

	// Enable uuid-ossp extension so uuid_generate_v4() exists for UUID defaults
	if err := db.Exec(`CREATE EXTENSION IF NOT EXISTS "uuid-ossp"`).Error; err != nil {
		panic("Failed to create uuid-ossp extension: " + err.Error())
	}

	// Create reminder_type enum for reminder_templates (required before creating reminder_templates table)
	if err := db.Exec(`
		DO $$ BEGIN
			CREATE TYPE reminder_type AS ENUM ('birthday', 'anniversary');
		EXCEPTION
			WHEN duplicate_object THEN null;
		END $$;
	`).Error; err != nil {
		panic("Failed to create reminder_type enum: " + err.Error())
	}

	// Create payment_status enum type for invoices (required before creating invoices table)
	if err := db.Exec(`
		DO $$ BEGIN
			CREATE TYPE payment_status AS ENUM ('unpaid', 'paid', 'partial');
		EXCEPTION
			WHEN duplicate_object THEN null;
		END $$;
	`).Error; err != nil {
		panic("Failed to create payment_status enum: " + err.Error())
	}

	//Optimize connection pool settings
	// sqlDB.SetMaxIdleConns(25)                 // Increase idle connections
	// sqlDB.SetMaxOpenConns(100)                // Increase max connections
	// sqlDB.SetConnMaxLifetime(5 * time.Minute) // Shorter lifetime
	// sqlDB.SetConnMaxIdleTime(time.Minute)     // Close idle connections faster

	DB = db
}
