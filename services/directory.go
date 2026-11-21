package services

import (
	"database/sql"
	"fmt"
	"strings"

	"pratham-hospitals/models"
)

type DirectoryService struct {
	db *sql.DB
}

func NewDirectoryService(db *sql.DB) *DirectoryService {
	return &DirectoryService{db: db}
}

// NormalizeString trims spaces and collapses multiple whitespace for comparisons
func NormalizeString(s string) string {
	fields := strings.Fields(strings.ToLower(strings.TrimSpace(s)))
	return strings.Join(fields, " ")
}

// SearchHospitals performs multi-filter, search, sorting, and pagination
func (s *DirectoryService) SearchHospitals(params models.SearchFilterParams) (*models.SearchResultWrapper, error) {
	if params.Page <= 0 {
		params.Page = 1
	}
	if params.PageSize <= 0 || params.PageSize > 100 {
		params.PageSize = 25
	}

	whereClauses := []string{"h.status = 'active'"}
	args := []interface{}{}
	argIdx := 1

	// Blocked filter: default public search excludes Blocked unless explicitly requested
	if !params.IncludeBlocked {
		whereClauses = append(whereClauses, "h.type != 'Exclusion'")
	}

	// Text query search: matches hospital name, location, district, state, doctor name or specialty name
	if strings.TrimSpace(params.Q) != "" {
		searchTerm := "%" + strings.TrimSpace(params.Q) + "%"
		qClause := fmt.Sprintf(`(
			h.hospital_name ILIKE $%d 
			OR COALESCE(h.location, '') ILIKE $%d 
			OR COALESCE(h.district, '') ILIKE $%d 
			OR COALESCE(h.state, '') ILIKE $%d
			OR EXISTS (
				SELECT 1 FROM hospital_specializations hs 
				JOIN specializations sp ON hs.specialization_id = sp.id 
				WHERE hs.hospital_id = h.id AND sp.canonical_name ILIKE $%d
			)
			OR EXISTS (
				SELECT 1 FROM hospital_doctors hd 
				JOIN doctors d ON hd.doctor_id = d.id 
				WHERE hd.hospital_id = h.id AND d.display_name ILIKE $%d
			)
		)`, argIdx, argIdx, argIdx, argIdx, argIdx, argIdx)
		whereClauses = append(whereClauses, qClause)
		args = append(args, searchTerm)
		argIdx++
	}

	// State filter
	if strings.TrimSpace(params.State) != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("h.state ILIKE $%d", argIdx))
		args = append(args, strings.TrimSpace(params.State))
		argIdx++
	}

	// District filter
	if strings.TrimSpace(params.District) != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("h.district ILIKE $%d", argIdx))
		args = append(args, strings.TrimSpace(params.District))
		argIdx++
	}

	// Location filter
	if strings.TrimSpace(params.Location) != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("h.location ILIKE $%d", argIdx))
		args = append(args, strings.TrimSpace(params.Location))
		argIdx++
	}

	// Hospital Type filter (OR logic within types)
	if len(params.Types) > 0 {
		typePlaceholders := []string{}
		for _, t := range params.Types {
			t = strings.TrimSpace(t)
			if t != "" {
				typePlaceholders = append(typePlaceholders, fmt.Sprintf("$%d", argIdx))
				args = append(args, t)
				argIdx++
			}
		}
		if len(typePlaceholders) > 0 {
			whereClauses = append(whereClauses, fmt.Sprintf("h.type IN (%s)", strings.Join(typePlaceholders, ", ")))
		}
	}

	// Specialization filter
	if strings.TrimSpace(params.Specialization) != "" {
		whereClauses = append(whereClauses, fmt.Sprintf(`EXISTS (
			SELECT 1 FROM hospital_specializations hs 
			JOIN specializations sp ON hs.specialization_id = sp.id 
			WHERE hs.hospital_id = h.id AND (sp.canonical_name ILIKE $%d OR sp.id::text = $%d)
		)`, argIdx, argIdx))
		args = append(args, strings.TrimSpace(params.Specialization))
		argIdx++
	}

	// Doctor Name filter
	if strings.TrimSpace(params.Doctor) != "" {
		whereClauses = append(whereClauses, fmt.Sprintf(`EXISTS (
			SELECT 1 FROM hospital_doctors hd 
			WHERE hd.hospital_id = h.id AND hd.display_name ILIKE $%d
		)`, argIdx))
		args = append(args, "%"+strings.TrimSpace(params.Doctor)+"%")
		argIdx++
	}

	whereSQL := strings.Join(whereClauses, " AND ")

	// Count total matching
	countSQL := fmt.Sprintf("SELECT COUNT(*) FROM hospitals h WHERE %s", whereSQL)
	var totalCount int
	err := s.db.QueryRow(countSQL, args...).Scan(&totalCount)
	if err != nil {
		return nil, fmt.Errorf("error counting hospitals: %w", err)
	}

	// Sort mapping
	orderCol := "h.hospital_name"
	orderDir := "ASC"
	if strings.ToUpper(params.SortOrder) == "DESC" {
		orderDir = "DESC"
	}
	switch strings.ToLower(params.SortBy) {
	case "source_sno", "sno":
		orderCol = "COALESCE(h.source_sno, 999999)"
	case "location":
		orderCol = "COALESCE(h.location, '')"
	case "district":
		orderCol = "COALESCE(h.district, '')"
	case "state":
		orderCol = "COALESCE(h.state, '')"
	case "type":
		orderCol = "h.type"
	case "created_at":
		orderCol = "h.created_at"
	default:
		orderCol = "h.hospital_name"
	}

	offset := (params.Page - 1) * params.PageSize

	selectSQL := fmt.Sprintf(`
		SELECT 
			h.id, h.source_sno, h.hospital_name, h.normalized_name, 
			h.address, h.location, h.district, h.state, 
			h.type, h.public_visibility, h.status, h.created_at, h.updated_at,
			(SELECT COUNT(*) FROM hospital_specializations hs WHERE hs.hospital_id = h.id) as specialty_count,
			(SELECT COUNT(*) FROM hospital_doctors hd WHERE hd.hospital_id = h.id) as doctor_count,
			COALESCE((
				SELECT string_agg(sp.canonical_name, ', ' ORDER BY sp.canonical_name) 
				FROM hospital_specializations hs 
				JOIN specializations sp ON hs.specialization_id = sp.id 
				WHERE hs.hospital_id = h.id
			), '') as specialties_summary
		FROM hospitals h
		WHERE %s
		ORDER BY %s %s
		LIMIT $%d OFFSET $%d
	`, whereSQL, orderCol, orderDir, argIdx, argIdx+1)

	args = append(args, params.PageSize, offset)

	rows, err := s.db.Query(selectSQL, args...)
	if err != nil {
		return nil, fmt.Errorf("error executing hospital search: %w", err)
	}
	defer rows.Close()

	hospitals := []models.Hospital{}
	for rows.Next() {
		var h models.Hospital
		var specSummary string
		err := rows.Scan(
			&h.ID, &h.SourceSno, &h.HospitalName, &h.NormalizedName,
			&h.Address, &h.Location, &h.District, &h.State,
			&h.Type, &h.PublicVisibility, &h.Status, &h.CreatedAt, &h.UpdatedAt,
			&h.SpecialtyCount, &h.DoctorCount, &specSummary,
		)
		if err != nil {
			return nil, fmt.Errorf("error scanning hospital: %w", err)
		}
		if specSummary != "" {
			h.SpecialtiesList = strings.Split(specSummary, ", ")
		} else {
			h.SpecialtiesList = []string{}
		}
		hospitals = append(hospitals, h)
	}

	totalPages := (totalCount + params.PageSize - 1) / params.PageSize
	if totalPages == 0 {
		totalPages = 1
	}

	return &models.SearchResultWrapper{
		TotalCount: totalCount,
		Page:       params.Page,
		PageSize:   params.PageSize,
		TotalPages: totalPages,
		Hospitals:  hospitals,
	}, nil
}

// GetHospitalDetail returns full hospital info, specialties, doctors, and dynamic fields
func (s *DirectoryService) GetHospitalDetail(id string) (*models.Hospital, []models.HospitalSpecialization, []models.HospitalDoctor, map[string]string, error) {
	var h models.Hospital
	query := `
		SELECT 
			id, source_sno, hospital_name, normalized_name, 
			address, location, district, state, 
			type, public_visibility, status, created_at, updated_at,
			(SELECT COUNT(*) FROM hospital_specializations hs WHERE hs.hospital_id = hospitals.id) as specialty_count,
			(SELECT COUNT(*) FROM hospital_doctors hd WHERE hd.hospital_id = hospitals.id) as doctor_count
		FROM hospitals
		WHERE id = $1
	`
	err := s.db.QueryRow(query, id).Scan(
		&h.ID, &h.SourceSno, &h.HospitalName, &h.NormalizedName,
		&h.Address, &h.Location, &h.District, &h.State,
		&h.Type, &h.PublicVisibility, &h.Status, &h.CreatedAt, &h.UpdatedAt,
		&h.SpecialtyCount, &h.DoctorCount,
	)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("hospital not found: %w", err)
	}

	// Fetch specialties
	specQuery := `
		SELECT 
			hs.id, hs.hospital_id, hs.specialization_id, sp.canonical_name, 
			hs.description, hs.status, hs.created_at, hs.updated_at,
			(
				SELECT COUNT(DISTINCT ds.hospital_doctor_id) 
				FROM doctor_specializations ds 
				WHERE ds.hospital_specialization_id = hs.id
			) as doctor_count
		FROM hospital_specializations hs
		JOIN specializations sp ON hs.specialization_id = sp.id
		WHERE hs.hospital_id = $1
		ORDER BY sp.canonical_name ASC
	`
	specRows, err := s.db.Query(specQuery, id)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("error querying specialties: %w", err)
	}
	defer specRows.Close()

	specialties := []models.HospitalSpecialization{}
	for specRows.Next() {
		var hs models.HospitalSpecialization
		if err := specRows.Scan(&hs.ID, &hs.HospitalID, &hs.SpecializationID, &hs.SpecialtyName, &hs.Description, &hs.Status, &hs.CreatedAt, &hs.UpdatedAt, &hs.DoctorCount); err != nil {
			return nil, nil, nil, nil, err
		}
		specialties = append(specialties, hs)
	}

	// Fetch affiliated doctors with their assigned specialties
	docQuery := `
		SELECT 
			hd.id, hd.hospital_id, h.hospital_name, hd.doctor_id, hd.display_name, 
			d.qualifications, hd.status, hd.needs_specialty_review, hd.created_at, hd.updated_at,
			COALESCE((
				SELECT string_agg(sp.canonical_name, ', ' ORDER BY sp.canonical_name)
				FROM doctor_specializations ds
				JOIN hospital_specializations hs ON ds.hospital_specialization_id = hs.id
				JOIN specializations sp ON hs.specialization_id = sp.id
				WHERE ds.hospital_doctor_id = hd.id
			), '') as specialties
		FROM hospital_doctors hd
		JOIN hospitals h ON hd.hospital_id = h.id
		JOIN doctors d ON hd.doctor_id = d.id
		WHERE hd.hospital_id = $1
		ORDER BY hd.display_name ASC
	`
	docRows, err := s.db.Query(docQuery, id)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("error querying doctors: %w", err)
	}
	defer docRows.Close()

	doctors := []models.HospitalDoctor{}
	for docRows.Next() {
		var hd models.HospitalDoctor
		var specStr string
		if err := docRows.Scan(
			&hd.ID, &hd.HospitalID, &hd.HospitalName, &hd.DoctorID, &hd.DisplayName,
			&hd.Qualifications, &hd.Status, &hd.NeedsSpecialtyReview, &hd.CreatedAt, &hd.UpdatedAt,
			&specStr,
		); err != nil {
			return nil, nil, nil, nil, err
		}
		if specStr != "" {
			hd.Specialties = strings.Split(specStr, ", ")
		} else {
			hd.Specialties = []string{}
		}
		doctors = append(doctors, hd)
	}

	// Fetch custom field values
	cfQuery := `
		SELECT cfd.label, cfv.value
		FROM custom_field_values cfv
		JOIN custom_field_definitions cfd ON cfv.definition_id = cfd.id
		WHERE cfv.entity_id = $1 AND cfd.active = true
	`
	cfRows, err := s.db.Query(cfQuery, id)
	customFields := make(map[string]string)
	if err == nil {
		defer cfRows.Close()
		for cfRows.Next() {
			var label, val string
			if err := cfRows.Scan(&label, &val); err == nil && val != "" {
				customFields[label] = val
			}
		}
	}

	return &h, specialties, doctors, customFields, nil
}

// GetGeographyHierarchy returns available States, Districts, and Locations with counts
func (s *DirectoryService) GetGeographyHierarchy() ([]models.GeoCountItem, []models.GeoCountItem, []models.GeoCountItem, error) {
	// States
	states := []models.GeoCountItem{}
	stateRows, err := s.db.Query(`
		SELECT COALESCE(state, 'Unassigned'), COUNT(*),
		       (SELECT COUNT(*) FROM hospital_doctors hd JOIN hospitals h2 ON hd.hospital_id = h2.id WHERE COALESCE(h2.state, 'Unassigned') = COALESCE(h.state, 'Unassigned'))
		FROM hospitals h
		WHERE h.status = 'active' AND h.type != 'Exclusion'
		GROUP BY state
		ORDER BY state ASC
	`)
	if err == nil {
		defer stateRows.Close()
		for stateRows.Next() {
			var item models.GeoCountItem
			if err := stateRows.Scan(&item.Name, &item.HospitalCount, &item.DoctorCount); err == nil {
				states = append(states, item)
			}
		}
	}

	// Districts
	districts := []models.GeoCountItem{}
	districtRows, err := s.db.Query(`
		SELECT COALESCE(district, 'Unassigned'), COALESCE(state, ''), COUNT(*),
		       (SELECT COUNT(*) FROM hospital_doctors hd JOIN hospitals h2 ON hd.hospital_id = h2.id WHERE COALESCE(h2.district, 'Unassigned') = COALESCE(h.district, 'Unassigned'))
		FROM hospitals h
		WHERE h.status = 'active' AND h.type != 'Exclusion'
		GROUP BY district, state
		ORDER BY district ASC
	`)
	if err == nil {
		defer districtRows.Close()
		for districtRows.Next() {
			var item models.GeoCountItem
			if err := districtRows.Scan(&item.Name, &item.ParentName, &item.HospitalCount, &item.DoctorCount); err == nil {
				districts = append(districts, item)
			}
		}
	}

	// Locations
	locations := []models.GeoCountItem{}
	locRows, err := s.db.Query(`
		SELECT COALESCE(location, 'Unassigned'), COALESCE(district, ''), COUNT(*),
		       (SELECT COUNT(*) FROM hospital_doctors hd JOIN hospitals h2 ON hd.hospital_id = h2.id WHERE COALESCE(h2.location, 'Unassigned') = COALESCE(h.location, 'Unassigned'))
		FROM hospitals h
		WHERE h.status = 'active' AND h.type != 'Exclusion'
		GROUP BY location, district
		ORDER BY location ASC
	`)
	if err == nil {
		defer locRows.Close()
		for locRows.Next() {
			var item models.GeoCountItem
			if err := locRows.Scan(&item.Name, &item.ParentName, &item.HospitalCount, &item.DoctorCount); err == nil {
				locations = append(locations, item)
			}
		}
	}

	return states, districts, locations, nil
}

// GetGlobalStats returns dashboard KPI metrics
func (s *DirectoryService) GetGlobalStats() (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	var totalHospitals, prathamCount, anchorCount, networkCount, exclusionCount int
	err := s.db.QueryRow(`
		SELECT 
			COUNT(*),
			COUNT(*) FILTER (WHERE type = 'Pratham'),
			COUNT(*) FILTER (WHERE type = 'Anchor'),
			COUNT(*) FILTER (WHERE type = 'Network'),
			COUNT(*) FILTER (WHERE type = 'Exclusion')
		FROM hospitals
		WHERE status = 'active'
	`).Scan(&totalHospitals, &prathamCount, &anchorCount, &networkCount, &exclusionCount)
	if err != nil {
		return nil, err
	}

	var blankAddress, blankDistrict, blankState, blankLocation int
	_ = s.db.QueryRow(`
		SELECT 
			COUNT(*) FILTER (WHERE address IS NULL OR address = ''),
			COUNT(*) FILTER (WHERE district IS NULL OR district = ''),
			COUNT(*) FILTER (WHERE state IS NULL OR state = ''),
			COUNT(*) FILTER (WHERE location IS NULL OR location = '')
		FROM hospitals
		WHERE status = 'active'
	`).Scan(&blankAddress, &blankDistrict, &blankState, &blankLocation)

	var totalSpecializations int
	_ = s.db.QueryRow("SELECT COUNT(*) FROM specializations").Scan(&totalSpecializations)

	var totalDoctors, unassignedDoctors int
	_ = s.db.QueryRow(`
		SELECT 
			COUNT(*),
			COUNT(*) FILTER (WHERE needs_specialty_review = true)
		FROM hospital_doctors
		WHERE status = 'active'
	`).Scan(&totalDoctors, &unassignedDoctors)

	stats["total_hospitals"] = totalHospitals
	stats["pratham_count"] = prathamCount
	stats["pratham_neo_count"] = anchorCount
	stats["network_count"] = networkCount
	stats["blocked_count"] = exclusionCount
	stats["total_specializations"] = totalSpecializations
	stats["total_doctors"] = totalDoctors
	stats["unassigned_doctors"] = unassignedDoctors
	stats["blank_address"] = blankAddress
	stats["blank_district"] = blankDistrict
	stats["blank_state"] = blankState
	stats["blank_location"] = blankLocation

	return stats, nil
}

// GetAllSpecializations returns list of all canonical specializations with counts
func (s *DirectoryService) GetAllSpecializations() ([]models.Specialization, error) {
	query := `
		SELECT 
			sp.id, sp.canonical_name, sp.normalized_name, sp.description,
			(SELECT COUNT(*) FROM hospital_specializations hs WHERE hs.specialization_id = sp.id) as hospital_count,
			(SELECT COUNT(DISTINCT ds.hospital_doctor_id) 
			 FROM doctor_specializations ds 
			 JOIN hospital_specializations hs ON ds.hospital_specialization_id = hs.id 
			 WHERE hs.specialization_id = sp.id) as doctor_count,
			sp.created_at, sp.updated_at
		FROM specializations sp
		ORDER BY sp.canonical_name ASC
	`
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	res := []models.Specialization{}
	for rows.Next() {
		var item models.Specialization
		if err := rows.Scan(&item.ID, &item.CanonicalName, &item.NormalizedName, &item.Description, &item.HospitalCount, &item.DoctorCount, &item.CreatedAt, &item.UpdatedAt); err == nil {
			res = append(res, item)
		}
	}
	return res, nil
}

