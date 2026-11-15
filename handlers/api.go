package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"pratham-hospitals/models"
	"pratham-hospitals/services"
)

type APIHandler struct {
	db         *sql.DB
	dirService *services.DirectoryService
}

func NewAPIHandler(db *sql.DB) *APIHandler {
	return &APIHandler{
		db:         db,
		dirService: services.NewDirectoryService(db),
	}
}

// HealthCheckHandler handles /health
func (h *APIHandler) HealthCheckHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"healthy","service":"pratham-hospitals"}`))
}

// CascadingGeoHandler handles /api/v1/geography/cascading
func (h *APIHandler) CascadingGeoHandler(w http.ResponseWriter, r *http.Request) {
	state := strings.TrimSpace(r.URL.Query().Get("state"))
	district := strings.TrimSpace(r.URL.Query().Get("district"))

	districts := []models.GeoCountItem{}
	if state != "" {
		rows, err := h.db.Query(`
			SELECT COALESCE(district, 'Unassigned'), COUNT(*)
			FROM hospitals
			WHERE status = 'active' AND type != 'Blocked' AND state ILIKE $1 AND district IS NOT NULL AND district != ''
			GROUP BY district
			ORDER BY district ASC
		`, state)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var item models.GeoCountItem
				if err := rows.Scan(&item.Name, &item.HospitalCount); err == nil {
					districts = append(districts, item)
				}
			}
		}
	}

	locations := []models.GeoCountItem{}
	locQuery := `
		SELECT COALESCE(location, 'Unassigned'), COUNT(*)
		FROM hospitals
		WHERE status = 'active' AND type != 'Blocked' AND location IS NOT NULL AND location != ''
	`
	args := []interface{}{}
	argIdx := 1
	if state != "" {
		locQuery += fmt.Sprintf(" AND state ILIKE $%d", argIdx)
		args = append(args, state)
		argIdx++
	}
	if district != "" {
		locQuery += fmt.Sprintf(" AND district ILIKE $%d", argIdx)
		args = append(args, district)
		argIdx++
	}
	locQuery += " GROUP BY location ORDER BY location ASC"

	rows, err := h.db.Query(locQuery, args...)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var item models.GeoCountItem
			if err := rows.Scan(&item.Name, &item.HospitalCount); err == nil {
				locations = append(locations, item)
			}
		}
	}

	resp := map[string]interface{}{
		"districts": districts,
		"locations": locations,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// SearchAPIHandler handles /api/v1/hospitals JSON endpoint
func (h *APIHandler) SearchAPIHandler(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	state := strings.TrimSpace(r.URL.Query().Get("state"))
	district := strings.TrimSpace(r.URL.Query().Get("district"))
	location := strings.TrimSpace(r.URL.Query().Get("location"))
	types := r.URL.Query()["type"]
	specialization := strings.TrimSpace(r.URL.Query().Get("specialization"))

	params := models.SearchFilterParams{
		Q:              q,
		State:          state,
		District:       district,
		Location:       location,
		Types:          types,
		Specialization: specialization,
		Page:           1,
		PageSize:       50,
		SortBy:         "name",
		SortOrder:      "ASC",
		IncludeBlocked: false,
	}

	result, err := h.dirService.SearchHospitals(params)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}
