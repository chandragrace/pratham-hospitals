package seed

import (
	"database/sql"
	"log"
	"os"

	"github.com/google/uuid"
	"pratham-hospitals/services"
)

func SeedInitialData(db *sql.DB) error {
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM hospitals").Scan(&count)
	if err != nil {
		return err
	}

	excelSvc := services.NewExcelService(db)

	if count == 0 {
		log.Println("Database is empty. Seeding 98 PRATHAM Chennai Zone hospitals from master dataset...")

		file, err := os.Open("data/primary_master_chennai.csv")
		if err == nil {
			defer file.Close()
			_, _, err = excelSvc.ParseHospitalMaster(file, "primary_master_chennai.csv", true, false)
			if err != nil {
				log.Printf("Warning: failed to seed primary hospitals: %v", err)
			} else {
				log.Println("Successfully seeded 98 Hospital Master records!")
			}
		} else {
			log.Printf("Could not open data/primary_master_chennai.csv: %v", err)
		}

		// Seed secondary data
		secFile, err := os.Open("data/secondary_specializations_doctors.csv")
		if err == nil {
			defer secFile.Close()
			_, _, err = excelSvc.ParseSecondaryUpload(secFile, "secondary_specializations_doctors.csv", false)
			if err != nil {
				log.Printf("Warning: failed to seed secondary records: %v", err)
			} else {
				log.Println("Successfully seeded initial Specialization & Doctor association records!")
			}
		}

		// Seed sample custom field definitions for demonstration
		seedCustomFields(db)
	}

	return nil
}

func seedCustomFields(db *sql.DB) {
	fields := []struct {
		Key      string
		Label    string
		Scope    string
		Type     string
		Required bool
		Search   bool
		Filter   bool
		Public   bool
		Options  string
	}{
		{"emergency_phone", "24/7 Emergency Helpline", "hospital", "short_text", false, true, false, true, "[]"},
		{"bed_capacity", "Total Bed Capacity", "hospital", "number", false, false, true, true, "[]"},
		{"nabh_accredited", "NABH Accredited", "hospital", "boolean", false, false, true, true, "[]"},
		{"opd_timings", "OPD Operating Hours", "hospital", "short_text", false, false, false, true, "[]"},
		{"experience_years", "Years of Experience", "doctor", "number", false, false, true, true, "[]"},
	}

	for _, f := range fields {
		_, _ = db.Exec(`
			INSERT INTO custom_field_definitions 
			(id, field_key, label, entity_scope, data_type, required, searchable, filterable, options, display_order, is_public, active)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9::jsonb, 1, $10, true)
			ON CONFLICT (field_key) DO NOTHING
		`, uuid.New().String(), f.Key, f.Label, f.Scope, f.Type, f.Required, f.Search, f.Filter, f.Options, f.Public)
	}

	// Add sample custom field values for popular hospitals
	var mgmID string
	_ = db.QueryRow("SELECT id FROM hospitals WHERE hospital_name ILIKE '%MGM Healthcare%' LIMIT 1").Scan(&mgmID)
	if mgmID != "" {
		var defEmergency, defBed, defNABH string
		_ = db.QueryRow("SELECT id FROM custom_field_definitions WHERE field_key = 'emergency_phone'").Scan(&defEmergency)
		_ = db.QueryRow("SELECT id FROM custom_field_definitions WHERE field_key = 'bed_capacity'").Scan(&defBed)
		_ = db.QueryRow("SELECT id FROM custom_field_definitions WHERE field_key = 'nabh_accredited'").Scan(&defNABH)

		if defEmergency != "" {
			_, _ = db.Exec("INSERT INTO custom_field_values (id, definition_id, entity_id, value) VALUES ($1, $2, $3, $4) ON CONFLICT DO NOTHING", uuid.New().String(), defEmergency, mgmID, "044-45242424")
		}
		if defBed != "" {
			_, _ = db.Exec("INSERT INTO custom_field_values (id, definition_id, entity_id, value) VALUES ($1, $2, $3, $4) ON CONFLICT DO NOTHING", uuid.New().String(), defBed, mgmID, "400")
		}
		if defNABH != "" {
			_, _ = db.Exec("INSERT INTO custom_field_values (id, definition_id, entity_id, value) VALUES ($1, $2, $3, $4) ON CONFLICT DO NOTHING", uuid.New().String(), defNABH, mgmID, "true")
		}
	}
}
