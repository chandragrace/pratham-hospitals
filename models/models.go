package models

import (
	"time"
)

// Hospital represents the core Hospital entity
type Hospital struct {
	ID               string    `json:"id"`
	SourceSno        *int      `json:"source_sno,omitempty"`
	HospitalName     string    `json:"hospital_name"`
	NormalizedName   string    `json:"normalized_name"`
	Address          *string   `json:"address,omitempty"`
	Location         *string   `json:"location,omitempty"`
	District         *string   `json:"district,omitempty"`
	State            *string   `json:"state,omitempty"`
	Type             string    `json:"type"` // Pratham, Pratham Neo, Network, Blocked
	PublicVisibility bool      `json:"public_visibility"`
	Status           string    `json:"status"` // active, inactive
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`

	// Aggregated fields for listings
	SpecialtyCount   int                  `json:"specialty_count"`
	DoctorCount      int                  `json:"doctor_count"`
	SpecialtiesList  []string             `json:"specialties_list,omitempty"`
	DoctorsList      []HospitalDoctorItem `json:"doctors_list,omitempty"`
	CustomFields     map[string]string    `json:"custom_fields,omitempty"`
}

// HospitalDoctorItem represents a doctor affiliated with a hospital in listings
type HospitalDoctorItem struct {
	DoctorID       string   `json:"doctor_id"`
	DisplayName    string   `json:"display_name"`
	Qualifications string   `json:"qualifications,omitempty"`
	Specialties    []string `json:"specialties,omitempty"`
}

// Specialization represents a canonical medical specialty
type Specialization struct {
	ID             string    `json:"id"`
	CanonicalName  string    `json:"canonical_name"`
	NormalizedName string    `json:"normalized_name"`
	Description    *string   `json:"description,omitempty"`
	HospitalCount  int       `json:"hospital_count,omitempty"`
	DoctorCount    int       `json:"doctor_count,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// HospitalSpecialization links a hospital to a specialty with optional custom description
type HospitalSpecialization struct {
	ID               string    `json:"id"`
	HospitalID       string    `json:"hospital_id"`
	SpecializationID string    `json:"specialization_id"`
	SpecialtyName    string    `json:"specialty_name,omitempty"`
	Description      *string   `json:"description,omitempty"`
	Status           string    `json:"status"`
	DoctorCount      int       `json:"doctor_count,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// Doctor represents a doctor profile
type Doctor struct {
	ID             string    `json:"id"`
	DisplayName    string    `json:"display_name"`
	NormalizedName string    `json:"normalized_name"`
	Qualifications *string   `json:"qualifications,omitempty"`
	HospitalCount  int       `json:"hospital_count,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	CustomFields   map[string]string `json:"custom_fields,omitempty"`
}

// HospitalDoctor represents doctor affiliation with a specific hospital
type HospitalDoctor struct {
	ID                   string   `json:"id"`
	HospitalID           string   `json:"hospital_id"`
	HospitalName         string   `json:"hospital_name,omitempty"`
	DoctorID             string   `json:"doctor_id"`
	DisplayName          string   `json:"display_name"`
	Qualifications       *string  `json:"qualifications,omitempty"`
	Status               string   `json:"status"`
	NeedsSpecialtyReview bool     `json:"needs_specialty_review"`
	Specialties          []string `json:"specialties,omitempty"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

// CustomFieldDefinition represents dynamic user-defined fields
type CustomFieldDefinition struct {
	ID           string   `json:"id"`
	FieldKey     string   `json:"field_key"`
	Label        string   `json:"label"`
	EntityScope  string   `json:"entity_scope"` // 'hospital', 'specialization', 'doctor'
	DataType     string   `json:"data_type"`    // 'short_text', 'long_text', 'number', 'date', 'single_select', 'multi_select', 'boolean'
	Required     bool     `json:"required"`
	Searchable   bool     `json:"searchable"`
	Filterable   bool     `json:"filterable"`
	Options      []string `json:"options,omitempty"`
	DisplayOrder int      `json:"display_order"`
	IsPublic     bool     `json:"is_public"`
	Active       bool     `json:"active"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// CustomFieldValue stores values for custom fields
type CustomFieldValue struct {
	ID           string `json:"id"`
	DefinitionID string `json:"definition_id"`
	FieldKey     string `json:"field_key,omitempty"`
	FieldLabel   string `json:"field_label,omitempty"`
	EntityID     string `json:"entity_id"`
	Value        string `json:"value"`
}

// ImportBatch tracks an Excel import session
type ImportBatch struct {
	ID           string    `json:"id"`
	FileName     string    `json:"file_name"`
	UploadType   string    `json:"upload_type"` // 'hospital_master', 'specialization_doctor'
	Uploader     string    `json:"uploader"`
	TotalRows    int       `json:"total_rows"`
	AcceptedRows int       `json:"accepted_rows"`
	CreatedRows  int       `json:"created_rows"`
	UpdatedRows  int       `json:"updated_rows"`
	SkippedRows  int       `json:"skipped_rows"`
	InvalidRows  int       `json:"invalid_rows"`
	ReviewRows   int       `json:"review_rows"`
	Status       string    `json:"status"` // 'completed', 'failed', 'preview'
	SummaryNotes string    `json:"summary_notes,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

// ImportRowLog stores row-by-row audit results
type ImportRowLog struct {
	ID                string                 `json:"id"`
	BatchID           string                 `json:"batch_id"`
	RowNumber         int                    `json:"row_number"`
	SourceData        map[string]interface{} `json:"source_data"`
	Status            string                 `json:"status"` // 'created', 'updated', 'skipped', 'invalid', 'needs_review'
	ErrorMessage      string                 `json:"error_message,omitempty"`
	RecommendedAction string                 `json:"recommended_action,omitempty"`
	CreatedAt         time.Time              `json:"created_at"`
}

// AdminUser represents an authenticated admin
type AdminUser struct {
	ID           string     `json:"id"`
	Username     string     `json:"username"`
	Email        string     `json:"email"`
	PasswordHash string     `json:"-"`
	Role         string     `json:"role"`
	Active       bool       `json:"active"`
	LastLogin    *time.Time `json:"last_login,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
}

// SearchFilterParams encapsulates search and filter query parameters
type SearchFilterParams struct {
	Q              string   `json:"q"`
	State          string   `json:"state"`
	District       string   `json:"district"`
	Location       string   `json:"location"`
	Types          []string `json:"types"`
	Specialization string   `json:"specialization"`
	Doctor         string   `json:"doctor"`
	Page           int      `json:"page"`
	PageSize       int      `json:"page_size"`
	SortBy         string   `json:"sort_by"`
	SortOrder      string   `json:"sort_order"`
	IncludeBlocked bool     `json:"include_blocked"`
}

// SearchResultWrapper wraps paginated search results
type SearchResultWrapper struct {
	TotalCount int        `json:"total_count"`
	Page       int        `json:"page"`
	PageSize   int        `json:"page_size"`
	TotalPages int        `json:"total_pages"`
	Hospitals  []Hospital `json:"hospitals"`
}

// GeoCountItem represents aggregated counts for State, District, Location
type GeoCountItem struct {
	Name          string `json:"name"`
	ParentName    string `json:"parent_name,omitempty"`
	StateName     string `json:"state_name,omitempty"`
	HospitalCount int    `json:"hospital_count"`
	DoctorCount   int    `json:"doctor_count"`
}

// SpecialtySearchParams encapsulates filter and search parameters for the Specialties directory
type SpecialtySearchParams struct {
	Q              string   `json:"q"`
	Specialties    []string `json:"specialties"`
	Doctor         string   `json:"doctor"`
	Qualification  string   `json:"qualification"`
	Hospital       string   `json:"hospital"`
	Types          []string `json:"types"`           // Pratham, Anchor, Network, Exclusion
	HospitalNature string   `json:"hospital_nature"` // govt, private, trust
	State          string   `json:"state"`
	District       string   `json:"district"`
	Location       string   `json:"location"`
	Pincode        string   `json:"pincode"`
	Tab            string   `json:"tab"`             // "specialties", "hospitals", "doctors"
	View           string   `json:"view"`            // "grid", "list", "table"
	SortBy         string   `json:"sort_by"`         // "name_asc", "name_desc", "hosp_count", "doc_count"
	Page           int      `json:"page"`
	PageSize       int      `json:"page_size"`
	IncludeBlocked bool     `json:"include_blocked"`
}

// SpecialtyDirectoryItem represents an enriched specialty in the directory
type SpecialtyDirectoryItem struct {
	ID              string   `json:"id"`
	CanonicalName   string   `json:"canonical_name"`
	NormalizedName  string   `json:"normalized_name"`
	Description     string   `json:"description"`
	HospitalCount   int      `json:"hospital_count"`
	DoctorCount     int      `json:"doctor_count"`
	SampleHospitals []string `json:"sample_hospitals,omitempty"`
}

// DoctorAffiliation represents a hospital where a doctor practices
type DoctorAffiliation struct {
	HospitalID   string `json:"hospital_id"`
	HospitalName string `json:"hospital_name"`
	Location     string `json:"location"`
	District     string `json:"district"`
	State        string `json:"state"`
	Type         string `json:"type"`
}

// DoctorDirectoryItem represents an enriched doctor profile in the directory
type DoctorDirectoryItem struct {
	ID             string              `json:"id"`
	DisplayName    string              `json:"display_name"`
	NormalizedName string              `json:"normalized_name"`
	Qualifications string              `json:"qualifications"`
	Specialties    []string            `json:"specialties"`
	Hospitals      []DoctorAffiliation `json:"hospitals"`
	HospitalCount  int                 `json:"hospital_count"`
}

// SpecialtySearchResult wraps results for the Medical Specialties Directory
type SpecialtySearchResult struct {
	Tab              string                   `json:"tab"`
	View             string                   `json:"view"`
	TotalCount       int                      `json:"total_count"`
	Page             int                      `json:"page"`
	PageSize         int                      `json:"page_size"`
	TotalPages       int                      `json:"total_pages"`
	Specialties      []SpecialtyDirectoryItem `json:"specialties,omitempty"`
	Hospitals        []Hospital               `json:"hospitals,omitempty"`
	Doctors          []DoctorDirectoryItem    `json:"doctors,omitempty"`
	TotalSpecialties int                      `json:"total_specialties"`
	TotalHospitals   int                      `json:"total_hospitals"`
	TotalDoctors     int                      `json:"total_doctors"`
	TotalLocations   int                      `json:"total_locations"`
}


