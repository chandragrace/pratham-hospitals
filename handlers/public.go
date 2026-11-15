package handlers

import (
	"database/sql"
	"html/template"
	"net/http"
	"strconv"
	"strings"

	"pratham-hospitals/models"
	"pratham-hospitals/services"
)

type PublicHandler struct {
	db          *sql.DB
	dirService  *services.DirectoryService
	tmpl        map[string]*template.Template
}

func NewPublicHandler(db *sql.DB, tmpl map[string]*template.Template) *PublicHandler {
	return &PublicHandler{
		db:         db,
		dirService: services.NewDirectoryService(db),
		tmpl:       tmpl,
	}
}

// DirectoryPageData holds data passed to directory.html
type DirectoryPageData struct {
	Title              string
	ActiveNav          string
	IsAdmin            bool
	AdminUser          string
	Result             *models.SearchResultWrapper
	Params             models.SearchFilterParams
	States             []models.GeoCountItem
	Districts          []models.GeoCountItem
	Locations          []models.GeoCountItem
	Specializations    []models.Specialization
	Stats              map[string]interface{}
	ShowBlockedSetting bool
	PageNumbers        []int
}

func (d *DirectoryPageData) HasType(t string) bool {
	for _, cur := range d.Params.Types {
		if cur == t {
			return true
		}
	}
	return false
}

// DirectoryHandler renders the main search directory
func (h *PublicHandler) DirectoryHandler(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	state := strings.TrimSpace(r.URL.Query().Get("state"))
	district := strings.TrimSpace(r.URL.Query().Get("district"))
	location := strings.TrimSpace(r.URL.Query().Get("location"))
	types := r.URL.Query()["type"]
	specialization := strings.TrimSpace(r.URL.Query().Get("specialization"))
	doctor := strings.TrimSpace(r.URL.Query().Get("doctor"))
	sortBy := strings.TrimSpace(r.URL.Query().Get("sort_by"))
	if sortBy == "" {
		sortBy = "name"
	}

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}

	// Check setting for showing blocked hospitals
	var showBlockedVal string
	_ = h.db.QueryRow("SELECT value FROM system_settings WHERE key = 'show_blocked_in_public'").Scan(&showBlockedVal)
	showBlocked := (showBlockedVal == "true")

	// If admin is logged in, allow them to view Blocked
	isAdmin, adminUser := CheckAdminSession(r)
	if isAdmin {
		showBlocked = true
	}

	params := models.SearchFilterParams{
		Q:              q,
		State:          state,
		District:       district,
		Location:       location,
		Types:          types,
		Specialization: specialization,
		Doctor:         doctor,
		Page:           page,
		PageSize:       20,
		SortBy:         sortBy,
		SortOrder:      "ASC",
		IncludeBlocked: showBlocked,
	}

	result, err := h.dirService.SearchHospitals(params)
	if err != nil {
		http.Error(w, "Search error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	states, districts, locations, _ := h.dirService.GetGeographyHierarchy()
	specialties, _ := h.dirService.GetAllSpecializations()
	stats, _ := h.dirService.GetGlobalStats()

	// Calculate pagination page numbers (show up to 7 buttons)
	pageNums := []int{}
	startPage := result.Page - 3
	if startPage < 1 {
		startPage = 1
	}
	endPage := startPage + 6
	if endPage > result.TotalPages {
		endPage = result.TotalPages
		startPage = endPage - 6
		if startPage < 1 {
			startPage = 1
		}
	}
	for i := startPage; i <= endPage; i++ {
		pageNums = append(pageNums, i)
	}

	data := DirectoryPageData{
		Title:              "Hospital Directory & Advanced Search",
		ActiveNav:          "home",
		IsAdmin:            isAdmin,
		AdminUser:          adminUser,
		Result:             result,
		Params:             params,
		States:             states,
		Districts:          districts,
		Locations:          locations,
		Specializations:    specialties,
		Stats:              stats,
		ShowBlockedSetting: showBlocked,
		PageNumbers:        pageNums,
	}

	t, ok := h.tmpl["directory"]
	if !ok {
		http.Error(w, "Template not found", http.StatusInternalServerError)
		return
	}
	_ = t.ExecuteTemplate(w, "layout.html", data)
}

// HospitalDetailPageData holds data for hospital_detail.html
type HospitalDetailPageData struct {
	Title             string
	ActiveNav         string
	IsAdmin           bool
	AdminUser         string
	Hospital          *models.Hospital
	Specialties       []models.HospitalSpecialization
	Doctors           []models.HospitalDoctor
	UnassignedDoctors []models.HospitalDoctor
	CustomFields      map[string]string
}

func (d *HospitalDetailPageData) DoctorInSpecialty(doc models.HospitalDoctor, specName string) bool {
	for _, s := range doc.Specialties {
		if strings.EqualFold(s, specName) {
			return true
		}
	}
	return false
}

// HospitalDetailHandler renders a single hospital page
func (h *PublicHandler) HospitalDetailHandler(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 3 || parts[2] == "" {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	id := parts[2]

	hosp, specialties, doctors, customFields, err := h.dirService.GetHospitalDetail(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	unassigned := []models.HospitalDoctor{}
	for _, doc := range doctors {
		if doc.NeedsSpecialtyReview || len(doc.Specialties) == 0 {
			unassigned = append(unassigned, doc)
		}
	}

	isAdmin, adminUser := CheckAdminSession(r)

	data := HospitalDetailPageData{
		Title:             hosp.HospitalName,
		ActiveNav:         "home",
		IsAdmin:           isAdmin,
		AdminUser:         adminUser,
		Hospital:          hosp,
		Specialties:       specialties,
		Doctors:           doctors,
		UnassignedDoctors: unassigned,
		CustomFields:      customFields,
	}

	t, ok := h.tmpl["hospital_detail"]
	if !ok {
		http.Error(w, "Template not found", http.StatusInternalServerError)
		return
	}
	_ = t.ExecuteTemplate(w, "layout.html", data)
}

// SpecializationDetailHandler renders specialty detail drilldown
func (h *PublicHandler) SpecializationDetailHandler(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 3 || parts[2] == "" {
		http.Redirect(w, r, "/browse/specializations", http.StatusFound)
		return
	}
	id := parts[2]

	var spec models.Specialization
	err := h.db.QueryRow(`
		SELECT id, canonical_name, normalized_name, description
		FROM specializations
		WHERE id::text = $1 OR normalized_name = $1
	`, id).Scan(&spec.ID, &spec.CanonicalName, &spec.NormalizedName, &spec.Description)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	// Fetch hospitals offering this specialty
	hospRows, err := h.db.Query(`
		SELECT h.id, h.hospital_name, h.location, h.district, h.state, h.type
		FROM hospitals h
		JOIN hospital_specializations hs ON hs.hospital_id = h.id
		WHERE hs.specialization_id = $1 AND h.status = 'active'
		ORDER BY h.hospital_name ASC
	`, spec.ID)
	hospitals := []models.Hospital{}
	if err == nil {
		defer hospRows.Close()
		for hospRows.Next() {
			var h models.Hospital
			if err := hospRows.Scan(&h.ID, &h.HospitalName, &h.Location, &h.District, &h.State, &h.Type); err == nil {
				hospitals = append(hospitals, h)
			}
		}
	}
	spec.HospitalCount = len(hospitals)

	isAdmin, adminUser := CheckAdminSession(r)

	data := struct {
		Title          string
		ActiveNav      string
		IsAdmin        bool
		AdminUser      string
		Specialization models.Specialization
		Hospitals      []models.Hospital
	}{
		Title:          spec.CanonicalName + " Hospitals",
		ActiveNav:      "spec",
		IsAdmin:        isAdmin,
		AdminUser:      adminUser,
		Specialization: spec,
		Hospitals:      hospitals,
	}

	t, ok := h.tmpl["specialization_detail"]
	if !ok {
		http.Error(w, "Template not found", http.StatusInternalServerError)
		return
	}
	_ = t.ExecuteTemplate(w, "layout.html", data)
}

// DoctorDetailHandler renders doctor profile drilldown
func (h *PublicHandler) DoctorDetailHandler(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 3 || parts[2] == "" {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	id := parts[2]

	var doc models.Doctor
	err := h.db.QueryRow("SELECT id, display_name, normalized_name, qualifications FROM doctors WHERE id::text = $1", id).Scan(&doc.ID, &doc.DisplayName, &doc.NormalizedName, &doc.Qualifications)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	type AffiliationView struct {
		HospitalID           string
		HospitalName         string
		HospitalLocation     string
		NeedsSpecialtyReview bool
		Specialties          []string
	}

	affRows, err := h.db.Query(`
		SELECT hd.hospital_id, h.hospital_name, COALESCE(h.location, ''), hd.needs_specialty_review,
		       COALESCE((
		           SELECT string_agg(sp.canonical_name, ', ' ORDER BY sp.canonical_name)
		           FROM doctor_specializations ds
		           JOIN hospital_specializations hs ON ds.hospital_specialization_id = hs.id
		           JOIN specializations sp ON hs.specialization_id = sp.id
		           WHERE ds.hospital_doctor_id = hd.id
		       ), '')
		FROM hospital_doctors hd
		JOIN hospitals h ON hd.hospital_id = h.id
		WHERE hd.doctor_id = $1
	`, doc.ID)

	affiliations := []AffiliationView{}
	if err == nil {
		defer affRows.Close()
		for affRows.Next() {
			var a AffiliationView
			var specStr string
			if err := affRows.Scan(&a.HospitalID, &a.HospitalName, &a.HospitalLocation, &a.NeedsSpecialtyReview, &specStr); err == nil {
				if specStr != "" {
					a.Specialties = strings.Split(specStr, ", ")
				}
				affiliations = append(affiliations, a)
			}
		}
	}

	isAdmin, adminUser := CheckAdminSession(r)

	data := struct {
		Title        string
		ActiveNav    string
		IsAdmin      bool
		AdminUser    string
		Doctor       models.Doctor
		Affiliations []AffiliationView
	}{
		Title:        doc.DisplayName,
		ActiveNav:    "home",
		IsAdmin:      isAdmin,
		AdminUser:    adminUser,
		Doctor:       doc,
		Affiliations: affiliations,
	}

	t, ok := h.tmpl["doctor_detail"]
	if !ok {
		http.Error(w, "Template not found", http.StatusInternalServerError)
		return
	}
	_ = t.ExecuteTemplate(w, "layout.html", data)
}

// BrowseGeoHandler renders the geographical drilldown page
func (h *PublicHandler) BrowseGeoHandler(w http.ResponseWriter, r *http.Request) {
	states, districts, locations, err := h.dirService.GetGeographyHierarchy()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	isAdmin, adminUser := CheckAdminSession(r)

	data := struct {
		Title     string
		ActiveNav string
		IsAdmin   bool
		AdminUser string
		States    []models.GeoCountItem
		Districts []models.GeoCountItem
		Locations []models.GeoCountItem
	}{
		Title:     "Browse by Geography - Hierarchy Drilldown",
		ActiveNav: "geo",
		IsAdmin:   isAdmin,
		AdminUser: adminUser,
		States:    states,
		Districts: districts,
		Locations: locations,
	}

	t, ok := h.tmpl["browse_geo"]
	if !ok {
		http.Error(w, "Template not found", http.StatusInternalServerError)
		return
	}
	_ = t.ExecuteTemplate(w, "layout.html", data)
}
