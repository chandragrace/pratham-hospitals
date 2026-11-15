package handlers

import (
	"database/sql"
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"pratham-hospitals/models"
	"pratham-hospitals/services"
)

type AdminHandler struct {
	db          *sql.DB
	dirService  *services.DirectoryService
	tmpl        map[string]*template.Template
}

func NewAdminHandler(db *sql.DB, tmpl map[string]*template.Template) *AdminHandler {
	return &AdminHandler{
		db:         db,
		dirService: services.NewDirectoryService(db),
		tmpl:       tmpl,
	}
}

// DashboardHandler renders the administration dashboard overview
func (h *AdminHandler) DashboardHandler(w http.ResponseWriter, r *http.Request) {
	stats, err := h.dirService.GetGlobalStats()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Fetch recent batches
	batchRows, err := h.db.Query(`
		SELECT id, file_name, upload_type, uploader, total_rows, accepted_rows, created_rows, updated_rows, skipped_rows, invalid_rows, review_rows, status, COALESCE(summary_notes, ''), created_at
		FROM import_batches
		ORDER BY created_at DESC
		LIMIT 10
	`)
	batches := []models.ImportBatch{}
	if err == nil {
		defer batchRows.Close()
		for batchRows.Next() {
			var b models.ImportBatch
			if err := batchRows.Scan(
				&b.ID, &b.FileName, &b.UploadType, &b.Uploader,
				&b.TotalRows, &b.AcceptedRows, &b.CreatedRows, &b.UpdatedRows,
				&b.SkippedRows, &b.InvalidRows, &b.ReviewRows, &b.Status,
				&b.SummaryNotes, &b.CreatedAt,
			); err == nil {
				batches = append(batches, b)
			}
		}
	}

	_, adminUser := CheckAdminSession(r)

	data := struct {
		Title     string
		ActiveNav string
		IsAdmin   bool
		AdminUser string
		Stats     map[string]interface{}
		Batches   []models.ImportBatch
	}{
		Title:     "Admin Dashboard - Pratham Directory",
		ActiveNav: "admin",
		IsAdmin:   true,
		AdminUser: adminUser,
		Stats:     stats,
		Batches:   batches,
	}

	t, ok := h.tmpl["admin_dashboard"]
	if !ok {
		http.Error(w, "Template not found", http.StatusInternalServerError)
		return
	}
	_ = t.ExecuteTemplate(w, "layout.html", data)
}

// HospitalsHandler handles hospital list and manual administration
func (h *AdminHandler) HospitalsHandler(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	hospType := strings.TrimSpace(r.URL.Query().Get("type"))

	whereClauses := []string{"1=1"}
	args := []interface{}{}
	argIdx := 1

	if q != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("(hospital_name ILIKE $%d OR COALESCE(location, '') ILIKE $%d)", argIdx, argIdx))
		args = append(args, "%"+q+"%")
		argIdx++
	}

	if hospType != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("type = $%d", argIdx))
		args = append(args, hospType)
		argIdx++
	}

	query := fmt.Sprintf(`
		SELECT 
			h.id, h.source_sno, h.hospital_name, h.normalized_name, 
			h.address, h.location, h.district, h.state, 
			h.type, h.public_visibility, h.status, h.created_at, h.updated_at,
			(SELECT COUNT(*) FROM hospital_specializations hs WHERE hs.hospital_id = h.id) as specialty_count,
			(SELECT COUNT(*) FROM hospital_doctors hd WHERE hd.hospital_id = h.id) as doctor_count
		FROM hospitals h
		WHERE %s
		ORDER BY h.hospital_name ASC
		LIMIT 100
	`, strings.Join(whereClauses, " AND "))

	rows, err := h.db.Query(query, args...)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	hospitals := []models.Hospital{}
	for rows.Next() {
		var h models.Hospital
		if err := rows.Scan(
			&h.ID, &h.SourceSno, &h.HospitalName, &h.NormalizedName,
			&h.Address, &h.Location, &h.District, &h.State,
			&h.Type, &h.PublicVisibility, &h.Status, &h.CreatedAt, &h.UpdatedAt,
			&h.SpecialtyCount, &h.DoctorCount,
		); err == nil {
			hospitals = append(hospitals, h)
		}
	}

	_, adminUser := CheckAdminSession(r)

	data := struct {
		Title     string
		ActiveNav string
		IsAdmin   bool
		AdminUser string
		Hospitals []models.Hospital
		Q         string
		Type      string
		Message   string
	}{
		Title:     "Manage Hospitals",
		ActiveNav: "admin",
		IsAdmin:   true,
		AdminUser: adminUser,
		Hospitals: hospitals,
		Q:         q,
		Type:      hospType,
		Message:   r.URL.Query().Get("msg"),
	}

	t := h.tmpl["admin_hospitals"]
	_ = t.ExecuteTemplate(w, "layout.html", data)
}

// HospitalSaveHandler handles create or update of hospital records
func (h *AdminHandler) HospitalSaveHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/admin/hospitals", http.StatusFound)
		return
	}

	id := strings.TrimSpace(r.FormValue("id"))
	name := strings.TrimSpace(r.FormValue("hospital_name"))
	snoStr := strings.TrimSpace(r.FormValue("source_sno"))
	location := strings.TrimSpace(r.FormValue("location"))
	district := strings.TrimSpace(r.FormValue("district"))
	state := strings.TrimSpace(r.FormValue("state"))
	hospType := strings.TrimSpace(r.FormValue("type"))

	if name == "" {
		http.Redirect(w, r, "/admin/hospitals?msg=Hospital+name+is+required", http.StatusFound)
		return
	}

	var sno *int
	if val, err := strconv.Atoi(snoStr); err == nil {
		sno = &val
	}

	normName := services.NormalizeString(name)

	if id == "" {
		// Insert
		newID := uuid.New().String()
		_, err := h.db.Exec(`
			INSERT INTO hospitals (id, source_sno, hospital_name, normalized_name, location, district, state, type, status, public_visibility)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'active', true)
		`, newID, sno, name, normName, location, district, state, hospType)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, "/admin/hospitals?msg=Hospital+created+successfully", http.StatusFound)
	} else {
		// Update
		_, err := h.db.Exec(`
			UPDATE hospitals
			SET hospital_name = $1, normalized_name = $2, source_sno = $3, location = $4, district = $5, state = $6, type = $7, updated_at = NOW()
			WHERE id = $8
		`, name, normName, sno, location, district, state, hospType, id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, "/admin/hospitals?msg=Hospital+updated+successfully", http.StatusFound)
	}
}

// HospitalToggleBlockHandler toggles Blocked canonical type
func (h *AdminHandler) HospitalToggleBlockHandler(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	action := r.URL.Query().Get("action")

	newType := "Blocked"
	if action == "unblock" {
		newType = "Pratham"
	}

	_, _ = h.db.Exec("UPDATE hospitals SET type = $1, updated_at = NOW() WHERE id = $2", newType, id)
	http.Redirect(w, r, "/admin/hospitals?msg=Hospital+status+updated", http.StatusFound)
}

// DoctorsHandler handles doctor management and unassigned specialty reviews
func (h *AdminHandler) DoctorsHandler(w http.ResponseWriter, r *http.Request) {
	filterReview := (r.URL.Query().Get("filter") == "review")

	query := `
		SELECT 
			hd.id, hd.hospital_id, h.hospital_name, COALESCE(h.location, ''), 
			hd.doctor_id, hd.display_name, d.qualifications, hd.status, hd.needs_specialty_review,
			COALESCE((
				SELECT string_agg(sp.canonical_name, ', ' ORDER BY sp.canonical_name)
				FROM doctor_specializations ds
				JOIN hospital_specializations hs ON ds.hospital_specialization_id = hs.id
				JOIN specializations sp ON hs.specialization_id = sp.id
				WHERE ds.hospital_doctor_id = hd.id
			), '')
		FROM hospital_doctors hd
		JOIN hospitals h ON hd.hospital_id = h.id
		JOIN doctors d ON hd.doctor_id = d.id
	`
	if filterReview {
		query += " WHERE hd.needs_specialty_review = true"
	}
	query += " ORDER BY hd.needs_specialty_review DESC, hd.display_name ASC LIMIT 150"

	rows, err := h.db.Query(query)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type DoctorAdminView struct {
		ID                   string
		HospitalID           string
		HospitalName         string
		HospitalLocation     string
		DoctorID             string
		DisplayName          string
		Qualifications       *string
		Status               string
		NeedsSpecialtyReview bool
		Specialties          []string
	}

	doctors := []DoctorAdminView{}
	for rows.Next() {
		var doc DoctorAdminView
		var specStr string
		if err := rows.Scan(
			&doc.ID, &doc.HospitalID, &doc.HospitalName, &doc.HospitalLocation,
			&doc.DoctorID, &doc.DisplayName, &doc.Qualifications, &doc.Status,
			&doc.NeedsSpecialtyReview, &specStr,
		); err == nil {
			if specStr != "" {
				doc.Specialties = strings.Split(specStr, ", ")
			}
			doctors = append(doctors, doc)
		}
	}

	var totalDoctors, reviewCount int
	_ = h.db.QueryRow("SELECT COUNT(*), COUNT(*) FILTER (WHERE needs_specialty_review = true) FROM hospital_doctors").Scan(&totalDoctors, &reviewCount)

	specialties, _ := h.dirService.GetAllSpecializations()
	_, adminUser := CheckAdminSession(r)

	data := struct {
		Title              string
		ActiveNav          string
		IsAdmin            bool
		AdminUser          string
		Doctors            []DoctorAdminView
		TotalCount         int
		ReviewCount        int
		OnlyReview         bool
		AllSpecializations []models.Specialization
		Message            string
	}{
		Title:              "Physician Review & Management",
		ActiveNav:          "admin",
		IsAdmin:            true,
		AdminUser:          adminUser,
		Doctors:            doctors,
		TotalCount:         totalDoctors,
		ReviewCount:        reviewCount,
		OnlyReview:         filterReview,
		AllSpecializations: specialties,
		Message:            r.URL.Query().Get("msg"),
	}

	t := h.tmpl["admin_doctors"]
	_ = t.ExecuteTemplate(w, "layout.html", data)
}

// DoctorAssignSpecialtyHandler assigns a specialty to a doctor affiliation
func (h *AdminHandler) DoctorAssignSpecialtyHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/admin/doctors", http.StatusFound)
		return
	}

	hdID := strings.TrimSpace(r.FormValue("hospital_doctor_id"))
	hospID := strings.TrimSpace(r.FormValue("hospital_id"))
	specID := strings.TrimSpace(r.FormValue("specialization_id"))
	desc := strings.TrimSpace(r.FormValue("description"))

	if hdID == "" || hospID == "" || specID == "" {
		http.Redirect(w, r, "/admin/doctors?msg=Missing+required+fields", http.StatusFound)
		return
	}

	tx, err := h.db.Begin()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	// 1. Link or update hospital_specialization
	var descVal *string
	if desc != "" {
		descVal = &desc
	}
	var hospSpecID string
	err = tx.QueryRow(`
		INSERT INTO hospital_specializations (hospital_id, specialization_id, description, status)
		VALUES ($1, $2, $3, 'active')
		ON CONFLICT (hospital_id, specialization_id) DO UPDATE SET updated_at = NOW()
		RETURNING id
	`, hospID, specID, descVal).Scan(&hospSpecID)
	if err != nil {
		http.Error(w, "Error linking hospital specialty: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// 2. Link doctor_specialization
	_, err = tx.Exec(`
		INSERT INTO doctor_specializations (hospital_doctor_id, hospital_specialization_id, status)
		VALUES ($1, $2, 'active')
		ON CONFLICT (hospital_doctor_id, hospital_specialization_id) DO NOTHING
	`, hdID, hospSpecID)
	if err != nil {
		http.Error(w, "Error linking doctor specialty: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// 3. Mark review cleared
	_, _ = tx.Exec("UPDATE hospital_doctors SET needs_specialty_review = false, updated_at = NOW() WHERE id = $1", hdID)

	_ = tx.Commit()
	http.Redirect(w, r, "/admin/doctors?msg=Specialty+assigned+successfully", http.StatusFound)
}

// CustomFieldsHandler handles dynamic field definition builder
func (h *AdminHandler) CustomFieldsHandler(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.Query(`
		SELECT id, field_key, label, entity_scope, data_type, required, searchable, filterable, display_order, is_public, active, created_at, updated_at
		FROM custom_field_definitions
		ORDER BY entity_scope ASC, display_order ASC
	`)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	fields := []models.CustomFieldDefinition{}
	for rows.Next() {
		var f models.CustomFieldDefinition
		if err := rows.Scan(
			&f.ID, &f.FieldKey, &f.Label, &f.EntityScope, &f.DataType,
			&f.Required, &f.Searchable, &f.Filterable, &f.DisplayOrder,
			&f.IsPublic, &f.Active, &f.CreatedAt, &f.UpdatedAt,
		); err == nil {
			fields = append(fields, f)
		}
	}

	_, adminUser := CheckAdminSession(r)

	data := struct {
		Title     string
		ActiveNav string
		IsAdmin   bool
		AdminUser string
		Fields    []models.CustomFieldDefinition
		Message   string
	}{
		Title:     "Dynamic Custom Field Builder",
		ActiveNav: "admin",
		IsAdmin:   true,
		AdminUser: adminUser,
		Fields:    fields,
		Message:   r.URL.Query().Get("msg"),
	}

	t := h.tmpl["admin_custom_fields"]
	_ = t.ExecuteTemplate(w, "layout.html", data)
}

// CustomFieldCreateHandler creates a new dynamic field definition
func (h *AdminHandler) CustomFieldCreateHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/admin/custom-fields", http.StatusFound)
		return
	}

	label := strings.TrimSpace(r.FormValue("label"))
	scope := strings.TrimSpace(r.FormValue("entity_scope"))
	dataType := strings.TrimSpace(r.FormValue("data_type"))
	optionsStr := strings.TrimSpace(r.FormValue("options"))
	required := (r.FormValue("required") == "true")
	searchable := (r.FormValue("searchable") == "true")
	filterable := (r.FormValue("filterable") == "true")
	isPublic := (r.FormValue("is_public") == "true")

	if label == "" {
		http.Redirect(w, r, "/admin/custom-fields?msg=Field+label+is+required", http.StatusFound)
		return
	}

	// Auto generate field_key from label
	fieldKey := strings.ToLower(strings.ReplaceAll(label, " ", "_"))
	fieldKey = strings.ReplaceAll(fieldKey, "-", "_")

	optionsJSON := "[]"
	if optionsStr != "" {
		opts := strings.Split(optionsStr, ",")
		cleanOpts := []string{}
		for _, o := range opts {
			clean := strings.TrimSpace(o)
			if clean != "" {
				cleanOpts = append(cleanOpts, fmt.Sprintf(`"%s"`, clean))
			}
		}
		optionsJSON = "[" + strings.Join(cleanOpts, ",") + "]"
	}

	_, err := h.db.Exec(`
		INSERT INTO custom_field_definitions 
		(id, field_key, label, entity_scope, data_type, required, searchable, filterable, options, display_order, is_public, active)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9::jsonb, 1, $10, true)
	`, uuid.New().String(), fieldKey, label, scope, dataType, required, searchable, filterable, optionsJSON, isPublic)

	if err != nil {
		http.Redirect(w, r, "/admin/custom-fields?msg=Error+creating+field:+"+err.Error(), http.StatusFound)
		return
	}

	http.Redirect(w, r, "/admin/custom-fields?msg=Custom+field+created+successfully", http.StatusFound)
}

// CustomFieldDeleteHandler deletes a dynamic field
func (h *AdminHandler) CustomFieldDeleteHandler(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id != "" {
		_, _ = h.db.Exec("DELETE FROM custom_field_definitions WHERE id = $1", id)
	}
	http.Redirect(w, r, "/admin/custom-fields?msg=Field+deleted", http.StatusFound)
}

// SettingsHandler handles configuration settings
func (h *AdminHandler) SettingsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		var showBlocked, siteTitle, contactEmail string
		_ = h.db.QueryRow("SELECT value FROM system_settings WHERE key = 'show_blocked_in_public'").Scan(&showBlocked)
		_ = h.db.QueryRow("SELECT value FROM system_settings WHERE key = 'site_title'").Scan(&siteTitle)
		_ = h.db.QueryRow("SELECT value FROM system_settings WHERE key = 'contact_email'").Scan(&contactEmail)

		_, adminUser := CheckAdminSession(r)

		data := struct {
			Title        string
			ActiveNav    string
			IsAdmin      bool
			AdminUser    string
			ShowBlocked  string
			SiteTitle    string
			ContactEmail string
			Message      string
		}{
			Title:        "System Settings",
			ActiveNav:    "admin",
			IsAdmin:      true,
			AdminUser:    adminUser,
			ShowBlocked:  showBlocked,
			SiteTitle:    siteTitle,
			ContactEmail: contactEmail,
			Message:      r.URL.Query().Get("msg"),
		}

		t := h.tmpl["admin_settings"]
		_ = t.ExecuteTemplate(w, "layout.html", data)
		return
	}

	if r.Method == http.MethodPost {
		showBlocked := "false"
		if r.FormValue("show_blocked_in_public") == "true" {
			showBlocked = "true"
		}
		siteTitle := strings.TrimSpace(r.FormValue("site_title"))
		contactEmail := strings.TrimSpace(r.FormValue("contact_email"))

		_, _ = h.db.Exec("UPDATE system_settings SET value = $1 WHERE key = 'show_blocked_in_public'", showBlocked)
		_, _ = h.db.Exec("UPDATE system_settings SET value = $1 WHERE key = 'site_title'", siteTitle)
		_, _ = h.db.Exec("UPDATE system_settings SET value = $1 WHERE key = 'contact_email'", contactEmail)

		http.Redirect(w, r, "/admin/settings?msg=Settings+saved+successfully", http.StatusFound)
	}
}
