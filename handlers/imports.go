package handlers

import (
	"database/sql"
	"fmt"
	"html/template"
	"net/http"
	"strings"

	"pratham-hospitals/models"
	"pratham-hospitals/services"
)

type ImportHandler struct {
	db       *sql.DB
	excelSvc *services.ExcelService
	tmpl     map[string]*template.Template
}

func NewImportHandler(db *sql.DB, tmpl map[string]*template.Template) *ImportHandler {
	return &ImportHandler{
		db:       db,
		excelSvc: services.NewExcelService(db),
		tmpl:     tmpl,
	}
}

// ImportMasterHandler handles Stage 1: Hospital Master Upload
func (h *ImportHandler) ImportMasterHandler(w http.ResponseWriter, r *http.Request) {
	_, adminUser := CheckAdminSession(r)

	if r.Method == http.MethodGet {
		data := struct {
			Title          string
			ActiveNav      string
			IsAdmin        bool
			AdminUser      string
			SuccessMessage string
			ErrorMessage   string
			Preview        *services.PreviewHospitalResult
		}{
			Title:     "Stage 1: Hospital Master Upload",
			ActiveNav: "admin",
			IsAdmin:   true,
			AdminUser: adminUser,
		}
		t := h.tmpl["admin_import_master"]
		_ = t.Execute(w, data)
		return
	}

	if r.Method == http.MethodPost {
		// Parse multipart form
		err := r.ParseMultipartForm(30 << 20) // 30 MB max
		if err != nil {
			http.Error(w, "File upload too large", http.StatusBadRequest)
			return
		}

		file, header, err := r.FormFile("file")
		if err != nil {
			data := struct {
				Title        string
				ActiveNav    string
				IsAdmin      bool
				AdminUser    string
				ErrorMessage string
				Preview      *services.PreviewHospitalResult
			}{
				Title:        "Stage 1: Hospital Master Upload",
				ActiveNav:    "admin",
				IsAdmin:      true,
				AdminUser:    adminUser,
				ErrorMessage: "Please select a valid .xlsx or .csv file to upload",
			}
			t := h.tmpl["admin_import_master"]
			_ = t.Execute(w, data)
			return
		}
		defer file.Close()

		action := r.FormValue("action")
		mapBlacklisted := (r.FormValue("map_blacklisted") == "true")
		dryRun := (action == "preview")

		preview, batchID, err := h.excelSvc.ParseHospitalMaster(file, header.Filename, mapBlacklisted, dryRun)
		if err != nil {
			data := struct {
				Title        string
				ActiveNav    string
				IsAdmin      bool
				AdminUser    string
				ErrorMessage string
				Preview      *services.PreviewHospitalResult
			}{
				Title:        "Stage 1: Hospital Master Upload",
				ActiveNav:    "admin",
				IsAdmin:      true,
				AdminUser:    adminUser,
				ErrorMessage: "Error processing workbook: " + err.Error(),
			}
			t := h.tmpl["admin_import_master"]
			_ = t.Execute(w, data)
			return
		}

		successMsg := ""
		if !dryRun {
			successMsg = fmt.Sprintf("Hospital Master imported successfully! Batch ID: %s. %d rows created, %d rows updated.",
				batchID, preview.NewRows, preview.UpdateRows)
		}

		data := struct {
			Title          string
			ActiveNav      string
			IsAdmin        bool
			AdminUser      string
			SuccessMessage string
			ErrorMessage   string
			Preview        *services.PreviewHospitalResult
		}{
			Title:          "Stage 1: Hospital Master Upload",
			ActiveNav:      "admin",
			IsAdmin:        true,
			AdminUser:      adminUser,
			SuccessMessage: successMsg,
			Preview:        preview,
		}

		t := h.tmpl["admin_import_master"]
		_ = t.Execute(w, data)
	}
}

// ImportSecondaryHandler handles Stage 2: Specialization & Doctor Upload (with Gating)
func (h *ImportHandler) ImportSecondaryHandler(w http.ResponseWriter, r *http.Request) {
	_, adminUser := CheckAdminSession(r)

	// Check master gating
	var hospCount int
	_ = h.db.QueryRow("SELECT COUNT(*) FROM hospitals").Scan(&hospCount)

	if r.Method == http.MethodGet {
		data := struct {
			Title               string
			ActiveNav           string
			IsAdmin             bool
			AdminUser           string
			HospitalMasterCount int
			SuccessMessage      string
			ErrorMessage        string
			Preview             *services.PreviewSecondaryResult
		}{
			Title:               "Stage 2: Specialization & Doctor Upload",
			ActiveNav:           "admin",
			IsAdmin:             true,
			AdminUser:           adminUser,
			HospitalMasterCount: hospCount,
		}
		t := h.tmpl["admin_import_secondary"]
		_ = t.Execute(w, data)
		return
	}

	if r.Method == http.MethodPost {
		if hospCount == 0 {
			http.Error(w, "Upload Hospital Master first", http.StatusForbidden)
			return
		}

		err := r.ParseMultipartForm(30 << 20)
		if err != nil {
			http.Error(w, "File upload too large", http.StatusBadRequest)
			return
		}

		file, header, err := r.FormFile("file")
		if err != nil {
			data := struct {
				Title               string
				ActiveNav           string
				IsAdmin             bool
				AdminUser           string
				HospitalMasterCount int
				ErrorMessage        string
				Preview             *services.PreviewSecondaryResult
			}{
				Title:               "Stage 2: Specialization & Doctor Upload",
				ActiveNav:           "admin",
				IsAdmin:             true,
				AdminUser:           adminUser,
				HospitalMasterCount: hospCount,
				ErrorMessage:        "Please select a valid file to upload",
			}
			t := h.tmpl["admin_import_secondary"]
			_ = t.Execute(w, data)
			return
		}
		defer file.Close()

		action := r.FormValue("action")
		dryRun := (action == "preview")

		preview, batchID, err := h.excelSvc.ParseSecondaryUpload(file, header.Filename, dryRun)
		if err != nil {
			data := struct {
				Title               string
				ActiveNav           string
				IsAdmin             bool
				AdminUser           string
				HospitalMasterCount int
				ErrorMessage        string
				Preview             *services.PreviewSecondaryResult
			}{
				Title:               "Stage 2: Specialization & Doctor Upload",
				ActiveNav:           "admin",
				IsAdmin:             true,
				AdminUser:           adminUser,
				HospitalMasterCount: hospCount,
				ErrorMessage:        "Error processing secondary workbook: " + err.Error(),
			}
			t := h.tmpl["admin_import_secondary"]
			_ = t.Execute(w, data)
			return
		}

		successMsg := ""
		if !dryRun {
			successMsg = fmt.Sprintf("Doctor and Specialization records committed successfully! Batch ID: %s. %d valid links, %d flagged for review.",
				batchID, preview.ValidRows, preview.ReviewRows)
		}

		data := struct {
			Title               string
			ActiveNav           string
			IsAdmin             bool
			AdminUser           string
			HospitalMasterCount int
			SuccessMessage      string
			ErrorMessage        string
			Preview             *services.PreviewSecondaryResult
		}{
			Title:               "Stage 2: Specialization & Doctor Upload",
			ActiveNav:           "admin",
			IsAdmin:             true,
			AdminUser:           adminUser,
			HospitalMasterCount: hospCount,
			SuccessMessage:      successMsg,
			Preview:             preview,
		}

		t := h.tmpl["admin_import_secondary"]
		_ = t.Execute(w, data)
	}
}

// ImportHistoryHandler lists import batches
func (h *ImportHandler) ImportHistoryHandler(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.Query(`
		SELECT id, file_name, upload_type, uploader, total_rows, accepted_rows, created_rows, updated_rows, skipped_rows, invalid_rows, review_rows, status, COALESCE(summary_notes, ''), created_at
		FROM import_batches
		ORDER BY created_at DESC
	`)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	batches := []models.ImportBatch{}
	for rows.Next() {
		var b models.ImportBatch
		if err := rows.Scan(
			&b.ID, &b.FileName, &b.UploadType, &b.Uploader,
			&b.TotalRows, &b.AcceptedRows, &b.CreatedRows, &b.UpdatedRows,
			&b.SkippedRows, &b.InvalidRows, &b.ReviewRows, &b.Status,
			&b.SummaryNotes, &b.CreatedAt,
		); err == nil {
			batches = append(batches, b)
		}
	}

	_, adminUser := CheckAdminSession(r)

	data := struct {
		Title     string
		ActiveNav string
		IsAdmin   bool
		AdminUser string
		Batches   []models.ImportBatch
	}{
		Title:     "Import Audit History",
		ActiveNav: "admin",
		IsAdmin:   true,
		AdminUser: adminUser,
		Batches:   batches,
	}

	t := h.tmpl["admin_import_history"]
	_ = t.Execute(w, data)
}

// DownloadErrorReportHandler generates and streams error Excel file
func (h *ImportHandler) DownloadErrorReportHandler(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 5 || parts[4] == "" {
		http.NotFound(w, r)
		return
	}
	batchID := parts[4]

	f, err := h.excelSvc.GenerateErrorReport(batchID)
	if err != nil {
		http.Error(w, "Error generating error report: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=Import_Error_Report_%s.xlsx", batchID))
	_ = f.Write(w)
}

// DownloadMasterTemplateHandler downloads Hospital Master template
func (h *ImportHandler) DownloadMasterTemplateHandler(w http.ResponseWriter, r *http.Request) {
	f, err := h.excelSvc.GenerateHospitalMasterTemplate()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", "attachment; filename=Pratham_Hospital_Master_Template.xlsx")
	_ = f.Write(w)
}

// DownloadSecondaryTemplateHandler downloads Doctor/Specialty template
func (h *ImportHandler) DownloadSecondaryTemplateHandler(w http.ResponseWriter, r *http.Request) {
	f, err := h.excelSvc.GenerateSecondaryTemplate()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", "attachment; filename=Pratham_Specialization_Doctor_Template.xlsx")
	_ = f.Write(w)
}
