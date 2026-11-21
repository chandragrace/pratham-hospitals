package services

import (
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/xuri/excelize/v2"
	"pratham-hospitals/models"
)

type ExcelService struct {
	db *sql.DB
}

func NewExcelService(db *sql.DB) *ExcelService {
	return &ExcelService{db: db}
}

// RowPreviewHospital holds preview data for hospital master rows
type RowPreviewHospital struct {
	RowNumber      int      `json:"row_number"`
	Action         string   `json:"action"` // 'add', 'modify', 'delete'
	SNo            *int     `json:"sno"`
	HospitalName   string   `json:"hospital_name"`
	Address        string   `json:"address"`
	Location       string   `json:"location"`
	District       string   `json:"district"`
	State          string   `json:"state"`
	Type           string   `json:"type"`
	CanonicalType  string   `json:"canonical_type"`
	Status         string   `json:"status"` // 'valid_new', 'valid_update', 'valid_delete', 'invalid', 'warning'
	Message        string   `json:"message"`
	ExistingID     string   `json:"existing_id,omitempty"`
}

// PreviewHospitalResult holds summary for hospital master preview
type PreviewHospitalResult struct {
	FileName        string                `json:"file_name"`
	TotalRows       int                   `json:"total_rows"`
	NewRows         int                   `json:"new_rows"`
	UpdateRows      int                   `json:"update_rows"`
	DeleteRows      int                   `json:"delete_rows"`
	WarningRows     int                   `json:"warning_rows"`
	InvalidRows     int                   `json:"invalid_rows"`
	DetectedColumns []string              `json:"detected_columns"`
	Rows            []RowPreviewHospital  `json:"rows"`
	HasBlacklisted  bool                  `json:"has_blacklisted"`
}

// ParseHospitalMaster parses an uploaded Excel or CSV file
func (s *ExcelService) ParseHospitalMaster(r io.Reader, filename string, mapBlacklistedToBlocked bool, dryRun bool) (*PreviewHospitalResult, string, error) {
	rows, err := readTabularData(r, filename)
	if err != nil {
		return nil, "", fmt.Errorf("error reading file: %w", err)
	}

	if len(rows) < 2 {
		return nil, "", fmt.Errorf("file does not contain enough data rows (minimum 1 header + 1 data row required)")
	}

	header := rows[0]
	colMap := detectHospitalColumns(header)

	if colMap["hospital_name"] == -1 {
		return nil, "", fmt.Errorf("required column 'Hospital Name' not detected in header")
	}

	preview := &PreviewHospitalResult{
		FileName:        filename,
		DetectedColumns: header,
		Rows:            []RowPreviewHospital{},
	}

	// Fetch existing hospitals for matching
	existingMap := make(map[string]models.Hospital)
	hRows, err := s.db.Query("SELECT id, normalized_name, COALESCE(location, '') FROM hospitals")
	if err == nil {
		defer hRows.Close()
		for hRows.Next() {
			var id, norm, loc string
			if err := hRows.Scan(&id, &norm, &loc); err == nil {
				key := norm + "|" + NormalizeString(loc)
				existingMap[key] = models.Hospital{ID: id, NormalizedName: norm}
				// Also key just by normalized name for fallback
				if _, exists := existingMap[norm]; !exists {
					existingMap[norm] = models.Hospital{ID: id, NormalizedName: norm}
				}
			}
		}
	}

	var batchID string
	var tx *sql.Tx
	if !dryRun {
		batchID = uuid.New().String()
		tx, err = s.db.Begin()
		if err != nil {
			return nil, "", fmt.Errorf("error starting transaction: %w", err)
		}
		defer tx.Rollback()

		// Pre-create batch record to satisfy foreign key in import_row_logs
		batchSQL := `
			INSERT INTO import_batches 
			(id, file_name, upload_type, uploader, total_rows, accepted_rows, created_rows, updated_rows, skipped_rows, invalid_rows, review_rows, status, summary_notes)
			VALUES ($1, $2, 'hospital_master', 'admin', 0, 0, 0, 0, 0, 0, 0, 'processing', 'Import started')
		`
		if _, err := tx.Exec(batchSQL, batchID, filename); err != nil {
			return nil, "", fmt.Errorf("error initializing import batch: %w", err)
		}
	}

	createdCount := 0
	updatedCount := 0
	invalidCount := 0
	warningCount := 0
	deletedCount := 0

	for i := 1; i < len(rows); i++ {
		row := rows[i]
		if isEmptyRow(row) {
			continue
		}

		rowNum := i + 1
		pRow := RowPreviewHospital{
			RowNumber: rowNum,
		}

		// Read Action column (default: "add")
		pRow.Action = "add"
		if colMap["action"] >= 0 && colMap["action"] < len(row) {
			actionVal := strings.ToLower(strings.TrimSpace(row[colMap["action"]]))
			switch actionVal {
			case "modify", "update", "edit":
				pRow.Action = "modify"
			case "delete", "remove":
				pRow.Action = "delete"
			default:
				pRow.Action = "add"
			}
		}

		// Read Hospital Name
		if colMap["hospital_name"] < len(row) {
			pRow.HospitalName = strings.TrimSpace(row[colMap["hospital_name"]])
		}

		if pRow.HospitalName == "" {
			pRow.Status = "invalid"
			pRow.Message = "Hospital Name is missing or empty"
			preview.InvalidRows++
			invalidCount++
			preview.Rows = append(preview.Rows, pRow)
			continue
		}

		// Read S.NO
		if colMap["sno"] >= 0 && colMap["sno"] < len(row) {
			snoStr := strings.TrimSpace(row[colMap["sno"]])
			if val, err := strconv.Atoi(snoStr); err == nil {
				pRow.SNo = &val
			}
		}

		// Read Address
		if colMap["address"] >= 0 && colMap["address"] < len(row) {
			pRow.Address = strings.TrimSpace(row[colMap["address"]])
		}

		// Read Location
		if colMap["location"] >= 0 && colMap["location"] < len(row) {
			pRow.Location = strings.TrimSpace(row[colMap["location"]])
		}

		// Read District
		if colMap["district"] >= 0 && colMap["district"] < len(row) {
			pRow.District = strings.TrimSpace(row[colMap["district"]])
		}

		// Read State
		if colMap["state"] >= 0 && colMap["state"] < len(row) {
			pRow.State = strings.TrimSpace(row[colMap["state"]])
		}

		// Read Type
		rawType := ""
		if colMap["type"] >= 0 && colMap["type"] < len(row) {
			rawType = strings.TrimSpace(row[colMap["type"]])
		}
		pRow.Type = rawType

		// Validate Type canonical values: Pratham, Anchor, Network, Exclusion
		canonicalType := "Pratham"
		lowerType := strings.ToLower(rawType)
		switch lowerType {
		case "pratham":
			canonicalType = "Pratham"
		case "anchor", "pratham neo", "prathamneo", "neo":
			canonicalType = "Anchor"
		case "network":
			canonicalType = "Network"
		case "exclusion", "blocked":
			canonicalType = "Exclusion"
		case "blacklisted":
			preview.HasBlacklisted = true
			canonicalType = "Exclusion"
			pRow.Message = "Mapped legacy 'Blacklisted' to canonical 'Exclusion'"
			pRow.Status = "warning"
			warningCount++
		default:
			if rawType == "" {
				canonicalType = "Pratham" // default
			} else {
				pRow.Status = "warning"
				pRow.Message = fmt.Sprintf("Unrecognized type '%s', defaulted to 'Pratham'", rawType)
				warningCount++
				canonicalType = "Pratham"
			}
		}
		pRow.CanonicalType = canonicalType

		normName := NormalizeString(pRow.HospitalName)
		normLoc := NormalizeString(pRow.Location)
		keyWithLoc := normName + "|" + normLoc

		// Check if exists
		existing, found := existingMap[keyWithLoc]
		if !found && normLoc == "" {
			existing, found = existingMap[normName]
		}

		// Handle action-based logic
		if pRow.Action == "delete" {
			if !found {
				pRow.Status = "invalid"
				pRow.Message = "Delete requested but hospital not found in master records"
				preview.InvalidRows++
				invalidCount++
			} else {
				pRow.ExistingID = existing.ID
				pRow.Status = "valid_delete"
				pRow.Message = "Hospital will be permanently deleted"
				preview.DeleteRows++
				deletedCount++
			}
		} else if found {
			pRow.ExistingID = existing.ID
			if pRow.Status == "" {
				pRow.Status = "valid_update"
				pRow.Message = "Existing hospital will be updated"
			}
			preview.UpdateRows++
			updatedCount++
		} else {
			if pRow.Status == "" {
				pRow.Status = "valid_new"
				pRow.Message = "New hospital will be created"
			}
			preview.NewRows++
			createdCount++
		}

		// Execute commit if not dry run
		if !dryRun && tx != nil {
			var hospID string
			if pRow.Action == "delete" && pRow.ExistingID != "" {
				// Hard delete: remove all related records then the hospital
				hospID = pRow.ExistingID
				_, _ = tx.Exec(`DELETE FROM doctor_specializations ds USING hospital_doctors hd WHERE hd.id = ds.hospital_doctor_id AND hd.hospital_id = $1`, hospID)
				_, _ = tx.Exec(`DELETE FROM hospital_doctors WHERE hospital_id = $1`, hospID)
				_, _ = tx.Exec(`DELETE FROM hospital_specializations WHERE hospital_id = $1`, hospID)
				_, err := tx.Exec(`DELETE FROM hospitals WHERE id = $1`, hospID)
				if err != nil {
					return nil, "", fmt.Errorf("error deleting hospital row %d: %w", rowNum, err)
				}
			} else if found {
				hospID = existing.ID
				updateSQL := `
					UPDATE hospitals 
					SET source_sno = COALESCE($1, source_sno),
					    address = CASE WHEN $2 != '' THEN $2 ELSE address END,
					    location = CASE WHEN $3 != '' THEN $3 ELSE location END,
					    district = CASE WHEN $4 != '' THEN $4 ELSE district END,
					    state = CASE WHEN $5 != '' THEN $5 ELSE state END,
					    type = $6,
					    updated_at = NOW()
					WHERE id = $7
				`
				_, err := tx.Exec(updateSQL, pRow.SNo, pRow.Address, pRow.Location, pRow.District, pRow.State, canonicalType, hospID)
				if err != nil {
					return nil, "", fmt.Errorf("error updating hospital row %d: %w", rowNum, err)
				}
			} else {
				hospID = uuid.New().String()
				insertSQL := `
					INSERT INTO hospitals 
					(id, source_sno, hospital_name, normalized_name, address, location, district, state, type, public_visibility, status)
					VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, true, 'active')
				`
				_, err := tx.Exec(insertSQL, hospID, pRow.SNo, pRow.HospitalName, normName, pRow.Address, pRow.Location, pRow.District, pRow.State, canonicalType)
				if err != nil {
					return nil, "", fmt.Errorf("error inserting hospital row %d: %w", rowNum, err)
				}
				// Update in-memory map
				existingMap[keyWithLoc] = models.Hospital{ID: hospID, NormalizedName: normName}
				existingMap[normName] = models.Hospital{ID: hospID, NormalizedName: normName}
			}

			// Insert Row Log
			logSQL := `
				INSERT INTO import_row_logs 
				(id, batch_id, row_number, source_data, status, error_message, recommended_action)
				VALUES ($1, $2, $3, $4, $5, $6, $7)
			`
			srcMap := map[string]string{
				"name":     pRow.HospitalName,
				"location": pRow.Location,
				"type":     rawType,
				"action":   pRow.Action,
			}
			srcBytes, _ := json.Marshal(srcMap)
			_, _ = tx.Exec(logSQL, uuid.New().String(), batchID, rowNum, string(srcBytes), pRow.Status, pRow.Message, "")
		}

		preview.Rows = append(preview.Rows, pRow)
	}

	preview.TotalRows = len(preview.Rows)

	if !dryRun && tx != nil {
		updateBatchSQL := `
			UPDATE import_batches 
			SET total_rows = $1, accepted_rows = $2, created_rows = $3, updated_rows = $4, invalid_rows = $5, review_rows = $6, status = 'completed', summary_notes = $7
			WHERE id = $8
		`
		summaryNotes := fmt.Sprintf("Imported %d new, updated %d, deleted %d hospitals, with %d warnings and %d invalid rows",
			createdCount, updatedCount, deletedCount, warningCount, invalidCount)
		_, err := tx.Exec(updateBatchSQL, preview.TotalRows, createdCount+updatedCount, createdCount, updatedCount, invalidCount, warningCount, summaryNotes, batchID)
		if err != nil {
			return nil, "", fmt.Errorf("error updating import batch: %w", err)
		}

		if err := tx.Commit(); err != nil {
			return nil, "", fmt.Errorf("error committing transaction: %w", err)
		}
	}

	return preview, batchID, nil
}

// RowPreviewSecondary holds preview data for doctor/specialization rows
type RowPreviewSecondary struct {
	RowNumber             int    `json:"row_number"`
	Action                string `json:"action"` // 'add', 'modify', 'delete'
	HospitalName          string `json:"hospital_name"`
	MatchedHospitalID     string `json:"matched_hospital_id,omitempty"`
	MatchedHospitalName   string `json:"matched_hospital_name,omitempty"`
	Specialization        string `json:"specialization"`
	Description           string `json:"description"`
	DoctorName            string `json:"doctor_name"`
	Status                string `json:"status"` // 'valid', 'unassigned_specialty', 'hospital_not_found', 'ambiguous', 'invalid', 'valid_delete'
	Message               string `json:"message"`
}

// PreviewSecondaryResult holds summary for secondary import preview
type PreviewSecondaryResult struct {
	FileName        string                 `json:"file_name"`
	TotalRows       int                    `json:"total_rows"`
	ValidRows       int                    `json:"valid_rows"`
	ReviewRows      int                    `json:"review_rows"`
	NotFoundRows    int                    `json:"not_found_rows"`
	InvalidRows     int                    `json:"invalid_rows"`
	DetectedColumns []string               `json:"detected_columns"`
	Rows            []RowPreviewSecondary  `json:"rows"`
}

// ParseSecondaryUpload parses Specialization & Doctor workbook
func (s *ExcelService) ParseSecondaryUpload(r io.Reader, filename string, dryRun bool) (*PreviewSecondaryResult, string, error) {
	// First check: Gating check. At least one hospital master must exist!
	var hospCount int
	err := s.db.QueryRow("SELECT COUNT(*) FROM hospitals").Scan(&hospCount)
	if err != nil || hospCount == 0 {
		return nil, "", fmt.Errorf("Upload the Hospital Master first. Specialization and Doctor records can only be linked to hospitals already in the system.")
	}

	rows, err := readTabularData(r, filename)
	if err != nil {
		return nil, "", fmt.Errorf("error reading file: %w", err)
	}

	if len(rows) < 2 {
		return nil, "", fmt.Errorf("file does not contain enough data rows (minimum 1 header + 1 data row required)")
	}

	header := rows[0]
	colMap := detectSecondaryColumns(header)

	if colMap["hospital_name"] == -1 {
		return nil, "", fmt.Errorf("required column 'Hospital Name' not detected in header")
	}

	preview := &PreviewSecondaryResult{
		FileName:        filename,
		DetectedColumns: header,
		Rows:            []RowPreviewSecondary{},
	}

	// Fetch all hospitals for normalized matching
	hospitalsList := []models.Hospital{}
	hRows, err := s.db.Query("SELECT id, hospital_name, normalized_name, COALESCE(location, ''), COALESCE(district, '') FROM hospitals")
	if err == nil {
		defer hRows.Close()
		for hRows.Next() {
			var h models.Hospital
			var loc, dist string
			if err := hRows.Scan(&h.ID, &h.HospitalName, &h.NormalizedName, &loc, &dist); err == nil {
				h.Location = &loc
				h.District = &dist
				hospitalsList = append(hospitalsList, h)
			}
		}
	}

	var batchID string
	var tx *sql.Tx
	if !dryRun {
		batchID = uuid.New().String()
		tx, err = s.db.Begin()
		if err != nil {
			return nil, "", fmt.Errorf("error starting transaction: %w", err)
		}
		defer tx.Rollback()

		// Pre-create batch record to satisfy foreign key in import_row_logs
		batchSQL := `
			INSERT INTO import_batches 
			(id, file_name, upload_type, uploader, total_rows, accepted_rows, created_rows, updated_rows, skipped_rows, invalid_rows, review_rows, status, summary_notes)
			VALUES ($1, $2, 'specialization_doctor', 'admin', 0, 0, 0, 0, 0, 0, 0, 'processing', 'Import started')
		`
		if _, err := tx.Exec(batchSQL, batchID, filename); err != nil {
			return nil, "", fmt.Errorf("error initializing import batch: %w", err)
		}
	}

	validCount := 0
	reviewCount := 0
	notFoundCount := 0
	invalidCount := 0

	for i := 1; i < len(rows); i++ {
		row := rows[i]
		if isEmptyRow(row) {
			continue
		}

		rowNum := i + 1
		pRow := RowPreviewSecondary{
			RowNumber: rowNum,
		}

		if colMap["hospital_name"] < len(row) {
			pRow.HospitalName = strings.TrimSpace(row[colMap["hospital_name"]])
		}
		if colMap["specialization"] >= 0 && colMap["specialization"] < len(row) {
			pRow.Specialization = strings.TrimSpace(row[colMap["specialization"]])
		}
		if colMap["description"] >= 0 && colMap["description"] < len(row) {
			pRow.Description = strings.TrimSpace(row[colMap["description"]])
		}
		if colMap["doctor_name"] >= 0 && colMap["doctor_name"] < len(row) {
			pRow.DoctorName = strings.TrimSpace(row[colMap["doctor_name"]])
		}

		// Read Action column (default: "add")
		pRow.Action = "add"
		if colMap["action"] >= 0 && colMap["action"] < len(row) {
			actionVal := strings.ToLower(strings.TrimSpace(row[colMap["action"]]))
			switch actionVal {
			case "modify", "update", "edit":
				pRow.Action = "modify"
			case "delete", "remove":
				pRow.Action = "delete"
			default:
				pRow.Action = "add"
			}
		}

		// Empty association check
		if pRow.HospitalName == "" {
			pRow.Status = "invalid"
			pRow.Message = "Hospital Name is required"
			preview.InvalidRows++
			invalidCount++
			preview.Rows = append(preview.Rows, pRow)
			continue
		}

		if pRow.Specialization == "" && pRow.DoctorName == "" {
			pRow.Status = "invalid"
			pRow.Message = "Both Specialization and Doctor Name are empty. Row skipped."
			preview.InvalidRows++
			invalidCount++
			preview.Rows = append(preview.Rows, pRow)
			continue
		}

		// Match Hospital Name
		normInput := NormalizeString(pRow.HospitalName)
		matches := []models.Hospital{}
		for _, h := range hospitalsList {
			if h.NormalizedName == normInput || strings.Contains(h.NormalizedName, normInput) || strings.Contains(normInput, h.NormalizedName) {
				matches = append(matches, h)
			}
		}

		if len(matches) == 0 {
			pRow.Status = "hospital_not_found"
			pRow.Message = fmt.Sprintf("Hospital '%s' not found in master records", pRow.HospitalName)
			preview.NotFoundRows++
			notFoundCount++
			preview.Rows = append(preview.Rows, pRow)
			continue
		} else if len(matches) > 1 {
			// Check exact match first
			exactMatchFound := false
			for _, m := range matches {
				if m.NormalizedName == normInput {
					pRow.MatchedHospitalID = m.ID
					pRow.MatchedHospitalName = m.HospitalName
					exactMatchFound = true
					break
				}
			}
			if !exactMatchFound {
				pRow.Status = "ambiguous"
				pRow.Message = fmt.Sprintf("Ambiguous hospital match (%d hospitals found)", len(matches))
				preview.InvalidRows++
				invalidCount++
				preview.Rows = append(preview.Rows, pRow)
				continue
			}
		} else {
			pRow.MatchedHospitalID = matches[0].ID
			pRow.MatchedHospitalName = matches[0].HospitalName
		}

		// Critical rule: If Doctor is present but Specialization is blank, flag for review!
		if pRow.DoctorName != "" && pRow.Specialization == "" {
			pRow.Status = "unassigned_specialty"
			pRow.Message = "Doctor attached with unassigned specialty. Needs specialty review."
			preview.ReviewRows++
			reviewCount++
		} else {
			pRow.Status = "valid"
			pRow.Message = "Valid relationship record"
			preview.ValidRows++
			validCount++
		}

		// If commit mode: execute DB operations
		if !dryRun && tx != nil {
			if pRow.Action == "delete" {
				// Hard delete: remove doctor-hospital link and specialization link
				if pRow.DoctorName != "" {
					var docID string
					_ = tx.QueryRow("SELECT id FROM doctors WHERE display_name = $1", pRow.DoctorName).Scan(&docID)
					if docID != "" {
						// Delete doctor_specializations for this hospital+doctor
						_, _ = tx.Exec(`
							DELETE FROM doctor_specializations ds
							USING hospital_doctors hd
							WHERE hd.id = ds.hospital_doctor_id
							  AND hd.hospital_id = $1
							  AND hd.doctor_id = $2
						`, pRow.MatchedHospitalID, docID)
						// Delete hospital_doctors link
						_, _ = tx.Exec(`
							DELETE FROM hospital_doctors
							WHERE hospital_id = $1 AND doctor_id = $2
						`, pRow.MatchedHospitalID, docID)
					}
				}
				if pRow.Specialization != "" {
					normSpec := NormalizeString(pRow.Specialization)
					var specID string
					_ = tx.QueryRow("SELECT id FROM specializations WHERE normalized_name = $1", normSpec).Scan(&specID)
					if specID != "" {
						// Delete all doctor_specializations linked to this hospital_specialization
						_, _ = tx.Exec(`
							DELETE FROM doctor_specializations ds
							USING hospital_specializations hs
							WHERE hs.id = ds.hospital_specialization_id
							  AND hs.hospital_id = $1
							  AND hs.specialization_id = $2
						`, pRow.MatchedHospitalID, specID)
						// Delete hospital_specializations link
						_, _ = tx.Exec(`
							DELETE FROM hospital_specializations
							WHERE hospital_id = $1 AND specialization_id = $2
						`, pRow.MatchedHospitalID, specID)
					}
				}
				pRow.Status = "deleted"
				pRow.Message = "Record deleted"
			} else {
			var specID string
			var hospSpecID string

			// 1. Create or get specialization if present
			if pRow.Specialization != "" {
				normSpec := NormalizeString(pRow.Specialization)
				err := tx.QueryRow(`
					INSERT INTO specializations (canonical_name, normalized_name) 
					VALUES ($1, $2)
					ON CONFLICT (normalized_name) DO UPDATE SET updated_at = NOW()
					RETURNING id
				`, pRow.Specialization, normSpec).Scan(&specID)
				if err != nil {
					return nil, "", fmt.Errorf("error creating specialization: %w", err)
				}

				// 2. Link Hospital to Specialization
				var descVal *string
				if pRow.Description != "" {
					descVal = &pRow.Description
				}
				err = tx.QueryRow(`
					INSERT INTO hospital_specializations (hospital_id, specialization_id, description, status)
					VALUES ($1, $2, $3, 'active')
					ON CONFLICT (hospital_id, specialization_id) 
					DO UPDATE SET description = COALESCE($3, hospital_specializations.description), updated_at = NOW()
					RETURNING id
				`, pRow.MatchedHospitalID, specID, descVal).Scan(&hospSpecID)
				if err != nil {
					return nil, "", fmt.Errorf("error creating hospital_specialization: %w", err)
				}
			}

			// 3. Create Doctor & Affiliation if present
			if pRow.DoctorName != "" {
				normDoc := NormalizeString(pRow.DoctorName)
				var docID string
				err := tx.QueryRow(`
					INSERT INTO doctors (display_name, normalized_name)
					VALUES ($1, $2)
					RETURNING id
				`, pRow.DoctorName, normDoc).Scan(&docID)
				if err != nil {
					// Doctor might already exist by display name
					_ = tx.QueryRow("SELECT id FROM doctors WHERE display_name = $1", pRow.DoctorName).Scan(&docID)
				}

				if docID != "" {
					needsReview := (pRow.Specialization == "")
					var hospDocID string
					err = tx.QueryRow(`
						INSERT INTO hospital_doctors (hospital_id, doctor_id, display_name, status, needs_specialty_review)
						VALUES ($1, $2, $3, 'active', $4)
						ON CONFLICT (hospital_id, doctor_id) 
						DO UPDATE SET needs_specialty_review = hospital_doctors.needs_specialty_review OR $4, updated_at = NOW()
						RETURNING id
					`, pRow.MatchedHospitalID, docID, pRow.DoctorName, needsReview).Scan(&hospDocID)

					if err == nil && hospDocID != "" && hospSpecID != "" {
						// Link Doctor to Specialization at this hospital
						_, _ = tx.Exec(`
							INSERT INTO doctor_specializations (hospital_doctor_id, hospital_specialization_id, status)
							VALUES ($1, $2, 'active')
							ON CONFLICT (hospital_doctor_id, hospital_specialization_id) DO NOTHING
						`, hospDocID, hospSpecID)
					}
				}
			}
			} // end else (add/modify)

			// Log row
			logSQL := `
				INSERT INTO import_row_logs 
				(id, batch_id, row_number, source_data, status, error_message, recommended_action)
				VALUES ($1, $2, $3, $4, $5, $6, $7)
			`
			srcMap := map[string]string{
				"hospital":  pRow.HospitalName,
				"specialty": pRow.Specialization,
				"doctor":    pRow.DoctorName,
			}
			srcBytes, _ := json.Marshal(srcMap)
			_, _ = tx.Exec(logSQL, uuid.New().String(), batchID, rowNum, string(srcBytes), pRow.Status, pRow.Message, "")
		}

		preview.Rows = append(preview.Rows, pRow)
	}

	preview.TotalRows = len(preview.Rows)

	if !dryRun && tx != nil {
		updateBatchSQL := `
			UPDATE import_batches 
			SET total_rows = $1, accepted_rows = $2, created_rows = $3, updated_rows = 0, invalid_rows = $4, review_rows = $5, status = 'completed', summary_notes = $6
			WHERE id = $7
		`
		summaryNotes := fmt.Sprintf("Processed %d rows: %d valid associations, %d unassigned specialty reviews, %d not found/invalid",
			preview.TotalRows, validCount, reviewCount, notFoundCount+invalidCount)
		_, err := tx.Exec(updateBatchSQL, preview.TotalRows, validCount+reviewCount, validCount+reviewCount, notFoundCount+invalidCount, reviewCount, summaryNotes, batchID)
		if err != nil {
			return nil, "", fmt.Errorf("error updating import batch: %w", err)
		}

		if err := tx.Commit(); err != nil {
			return nil, "", fmt.Errorf("error committing transaction: %w", err)
		}
	}

	return preview, batchID, nil
}

// GenerateHospitalMasterTemplate generates an Excel workbook template
func (s *ExcelService) GenerateHospitalMasterTemplate() (*excelize.File, error) {
	f := excelize.NewFile()
	sheet := "Hospital_Master"
	f.SetSheetName("Sheet1", sheet)

	headers := []string{"Action", "S.NO", "Hospital Name", "Address", "Location", "District", "State", "Type"}
	for colIdx, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(colIdx+1, 1)
		f.SetCellValue(sheet, cell, h)
	}

	// Sample rows
	samples := [][]interface{}{
		{"add", 1, "Apollo Speciality Hospitals", "21 Greams Lane", "Thousand Lights", "Chennai", "Tamil Nadu", "Pratham"},
		{"modify", 2, "Kauvery Hospital", "", "Kovilambakkam", "Chengalpattu", "Tamil Nadu", "Anchor"},
		{"delete", 3, "Lifeline Multispeciality", "", "Adyar", "Chennai", "Tamil Nadu", "Network"},
	}
	for rowIdx, row := range samples {
		for colIdx, val := range row {
			cell, _ := excelize.CoordinatesToCellName(colIdx+1, rowIdx+2)
			f.SetCellValue(sheet, cell, val)
		}
	}

	// Style header
	style, _ := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true, Color: "#FFFFFF"},
		Fill: excelize.Fill{Type: "pattern", Color: []string{"#375EBC"}, Pattern: 1},
	})
	f.SetRowStyle(sheet, 1, 1, style)
	return f, nil
}

// GenerateSecondaryTemplate generates the doctor/specialization Excel template
func (s *ExcelService) GenerateSecondaryTemplate() (*excelize.File, error) {
	f := excelize.NewFile()
	sheet := "Specialization_Doctor"
	f.SetSheetName("Sheet1", sheet)

	headers := []string{"Action", "Hospital Name", "Specialization", "Description", "Doctor Name"}
	for colIdx, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(colIdx+1, 1)
		f.SetCellValue(sheet, cell, h)
	}

	samples := [][]interface{}{
		{"add", "Apollo Speciality Hospitals", "Cardiology", "Cardio Robotic Surgery", "Dr. Rajesh Kumar MBBS MD DM"},
		{"add", "Apollo Speciality Hospitals", "Cardiology", "", "Dr. Priya Ramanathan MBBS DNB"},
		{"modify", "Apollo Speciality Hospitals", "", "", "Dr. R. Vijay Anand MBBS MS"},
		{"delete", "Kauvery Hospital", "Neurology", "Comprehensive Stroke Care", "Dr. K. Venkatesh MBBS MD DM"},
	}
	for rowIdx, row := range samples {
		for colIdx, val := range row {
			cell, _ := excelize.CoordinatesToCellName(colIdx+1, rowIdx+2)
			f.SetCellValue(sheet, cell, val)
		}
	}

	style, _ := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true, Color: "#FFFFFF"},
		Fill: excelize.Fill{Type: "pattern", Color: []string{"#274DA8"}, Pattern: 1},
	})
	f.SetRowStyle(sheet, 1, 1, style)
	return f, nil
}

// GenerateErrorReport generates an Excel file containing failed/review rows from an import batch
func (s *ExcelService) GenerateErrorReport(batchID string) (*excelize.File, error) {
	f := excelize.NewFile()
	sheet := "Error_Report"
	f.SetSheetName("Sheet1", sheet)

	headers := []string{"Row Number", "Status", "Error / Review Message", "Recommended Action", "Source Data"}
	for colIdx, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(colIdx+1, 1)
		f.SetCellValue(sheet, cell, h)
	}

	rows, err := s.db.Query(`
		SELECT row_number, status, COALESCE(error_message, ''), COALESCE(recommended_action, ''), source_data::text
		FROM import_row_logs
		WHERE batch_id = $1 AND status != 'created' AND status != 'updated'
		ORDER BY row_number ASC
	`, batchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	rowIdx := 2
	for rows.Next() {
		var rowNum int
		var status, errMsg, recAction, srcData string
		if err := rows.Scan(&rowNum, &status, &errMsg, &recAction, &srcData); err == nil {
			f.SetCellValue(sheet, fmt.Sprintf("A%d", rowIdx), rowNum)
			f.SetCellValue(sheet, fmt.Sprintf("B%d", rowIdx), status)
			f.SetCellValue(sheet, fmt.Sprintf("C%d", rowIdx), errMsg)
			f.SetCellValue(sheet, fmt.Sprintf("D%d", rowIdx), recAction)
			f.SetCellValue(sheet, fmt.Sprintf("E%d", rowIdx), srcData)
			rowIdx++
		}
	}

	style, _ := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true, Color: "#FFFFFF"},
		Fill: excelize.Fill{Type: "pattern", Color: []string{"#DC2626"}, Pattern: 1},
	})
	f.SetRowStyle(sheet, 1, 1, style)
	return f, nil
}

// Helper functions for reading tabular formats
func readTabularData(r io.Reader, filename string) ([][]string, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	if ext == ".csv" {
		reader := csv.NewReader(r)
		reader.LazyQuotes = true
		reader.FieldsPerRecord = -1
		return reader.ReadAll()
	}

	// Try reading as XLSX
	f, err := excelize.OpenReader(r)
	if err != nil {
		return nil, fmt.Errorf("unable to read Excel file: %w", err)
	}
	defer f.Close()

	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return nil, fmt.Errorf("no sheets found in workbook")
	}

	return f.GetRows(sheets[0])
}

func detectHospitalColumns(header []string) map[string]int {
	m := map[string]int{
		"action":        -1,
		"sno":           -1,
		"hospital_name": -1,
		"address":       -1,
		"location":      -1,
		"district":      -1,
		"state":         -1,
		"type":          -1,
	}

	for idx, col := range header {
		clean := strings.ToLower(strings.TrimSpace(col))
		clean = strings.ReplaceAll(clean, ".", "")
		clean = strings.ReplaceAll(clean, "_", "")
		clean = strings.ReplaceAll(clean, " ", "")

		if strings.Contains(clean, "action") || strings.Contains(clean, "tag") || strings.Contains(clean, "operation") {
			m["action"] = idx
		} else if strings.Contains(clean, "sno") || strings.Contains(clean, "serial") {
			m["sno"] = idx
		} else if strings.Contains(clean, "hospitalname") || clean == "hospital" || clean == "name" {
			m["hospital_name"] = idx
		} else if strings.Contains(clean, "address") {
			m["address"] = idx
		} else if strings.Contains(clean, "location") || strings.Contains(clean, "area") || strings.Contains(clean, "locality") {
			m["location"] = idx
		} else if strings.Contains(clean, "district") {
			m["district"] = idx
		} else if strings.Contains(clean, "state") {
			m["state"] = idx
		} else if strings.Contains(clean, "type") || strings.Contains(clean, "category") {
			m["type"] = idx
		}
	}
	return m
}

func detectSecondaryColumns(header []string) map[string]int {
	m := map[string]int{
		"action":         -1,
		"hospital_name":  -1,
		"specialization": -1,
		"description":    -1,
		"doctor_name":    -1,
	}

	for idx, col := range header {
		clean := strings.ToLower(strings.TrimSpace(col))
		clean = strings.ReplaceAll(clean, ".", "")
		clean = strings.ReplaceAll(clean, "_", "")
		clean = strings.ReplaceAll(clean, " ", "")

		if strings.Contains(clean, "action") || strings.Contains(clean, "tag") || strings.Contains(clean, "operation") {
			m["action"] = idx
		} else if strings.Contains(clean, "hospital") {
			m["hospital_name"] = idx
		} else if strings.Contains(clean, "special") || clean == "dept" || clean == "department" {
			m["specialization"] = idx
		} else if strings.Contains(clean, "desc") {
			m["description"] = idx
		} else if strings.Contains(clean, "doctor") || strings.Contains(clean, "physician") {
			m["doctor_name"] = idx
		}
	}
	return m
}

func isEmptyRow(row []string) bool {
	for _, cell := range row {
		if strings.TrimSpace(cell) != "" {
			return false
		}
	}
	return true
}
