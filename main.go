package main

import (
	"context"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"pratham-hospitals/config"
	"pratham-hospitals/handlers"
	"pratham-hospitals/seed"
)

func main() {
	log.Println("Starting PRATHAM HOSPITAL DIRECTORY Web Application...")

	cfg := config.LoadConfig()

	// Connect to Database
	db, err := config.ConnectDB(cfg)
	if err != nil {
		log.Fatalf("Database connection failed: %v", err)
	}
	defer db.Close()

	// Seed initial 98 Chennai Zone hospitals and doctor associations if empty
	if err := seed.SeedInitialData(db); err != nil {
		log.Printf("Seed warning: %v", err)
	}

	// Initialize Templates
	tmpl, err := initTemplates()
	if err != nil {
		log.Fatalf("Template initialization failed: %v", err)
	}

	// Handlers
	publicH := handlers.NewPublicHandler(db, tmpl)
	authH := handlers.NewAuthHandler(db, tmpl)
	adminH := handlers.NewAdminHandler(db, tmpl)
	importH := handlers.NewImportHandler(db, tmpl)
	apiH := handlers.NewAPIHandler(db)

	mux := http.NewServeMux()

	// Static files
	staticDir := filepath.Join(".", "static")
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(staticDir))))

	// Health check
	mux.HandleFunc("/health", apiH.HealthCheckHandler)

	// API endpoints
	mux.HandleFunc("/api/v1/geography/cascading", apiH.CascadingGeoHandler)
	mux.HandleFunc("/api/v1/hospitals", apiH.SearchAPIHandler)

	// Public Web routes
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			publicH.DirectoryHandler(w, r)
			return
		}
		if len(r.URL.Path) >= 10 && r.URL.Path[:10] == "/hospital/" {
			publicH.HospitalDetailHandler(w, r)
			return
		}
		if len(r.URL.Path) >= 16 && r.URL.Path[:16] == "/specialization/" {
			publicH.SpecializationDetailHandler(w, r)
			return
		}
		if len(r.URL.Path) >= 8 && r.URL.Path[:8] == "/doctor/" {
			publicH.DoctorDetailHandler(w, r)
			return
		}
		if len(r.URL.Path) >= 15 && r.URL.Path[:15] == "/browse/states/" {
			stateName := r.URL.Path[15:]
			http.Redirect(w, r, "/?state="+stateName, http.StatusFound)
			return
		}
		if len(r.URL.Path) >= 18 && r.URL.Path[:18] == "/browse/districts/" {
			distName := r.URL.Path[18:]
			http.Redirect(w, r, "/?district="+distName, http.StatusFound)
			return
		}
		if len(r.URL.Path) >= 18 && r.URL.Path[:18] == "/browse/locations/" {
			locName := r.URL.Path[18:]
			http.Redirect(w, r, "/?location="+locName, http.StatusFound)
			return
		}
		if r.URL.Path == "/browse/states" || r.URL.Path == "/browse/specializations" {
			publicH.BrowseGeoHandler(w, r)
			return
		}

		// Fallback to directory
		publicH.DirectoryHandler(w, r)
	})

	// Auth routes
	mux.HandleFunc("/admin/login", authH.LoginHandler)
	mux.HandleFunc("/admin/logout", authH.LogoutHandler)

	// Admin protected routes
	mux.HandleFunc("/admin", handlers.RequireAdmin(adminH.DashboardHandler))
	mux.HandleFunc("/admin/dashboard", handlers.RequireAdmin(adminH.DashboardHandler))
	mux.HandleFunc("/admin/hospitals", handlers.RequireAdmin(adminH.HospitalsHandler))
	mux.HandleFunc("/admin/hospitals/save", handlers.RequireAdmin(adminH.HospitalSaveHandler))
	mux.HandleFunc("/admin/hospitals/toggle-block", handlers.RequireAdmin(adminH.HospitalToggleBlockHandler))
	mux.HandleFunc("/admin/doctors", handlers.RequireAdmin(adminH.DoctorsHandler))
	mux.HandleFunc("/admin/doctors/assign-specialty", handlers.RequireAdmin(adminH.DoctorAssignSpecialtyHandler))
	mux.HandleFunc("/admin/custom-fields", handlers.RequireAdmin(adminH.CustomFieldsHandler))
	mux.HandleFunc("/admin/custom-fields/create", handlers.RequireAdmin(adminH.CustomFieldCreateHandler))
	mux.HandleFunc("/admin/custom-fields/delete", handlers.RequireAdmin(adminH.CustomFieldDeleteHandler))
	mux.HandleFunc("/admin/settings", handlers.RequireAdmin(adminH.SettingsHandler))
	mux.HandleFunc("/admin/settings/save", handlers.RequireAdmin(adminH.SettingsHandler))

	// Admin Import routes
	mux.HandleFunc("/admin/import/master", handlers.RequireAdmin(importH.ImportMasterHandler))
	mux.HandleFunc("/admin/import/secondary", handlers.RequireAdmin(importH.ImportSecondaryHandler))
	mux.HandleFunc("/admin/import/history", handlers.RequireAdmin(importH.ImportHistoryHandler))
	mux.HandleFunc("/admin/import/error-report/", handlers.RequireAdmin(importH.DownloadErrorReportHandler))
	mux.HandleFunc("/admin/template/master", handlers.RequireAdmin(importH.DownloadMasterTemplateHandler))
	mux.HandleFunc("/admin/template/secondary", handlers.RequireAdmin(importH.DownloadSecondaryTemplateHandler))

	server := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      securityHeadersMiddleware(mux),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Server runner
	go func() {
		log.Printf("Server listening on http://0.0.0.0:%s", cfg.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server gracefully...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Printf("Server forced shutdown: %v", err)
	}
	log.Println("Server exiting.")
}

func securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		next.ServeHTTP(w, r)
	})
}

func initTemplates() (map[string]*template.Template, error) {
	templates := make(map[string]*template.Template)

	funcMap := template.FuncMap{
		"add": func(a, b int) int { return a + b },
		"sub": func(a, b int) int { return a - b },
		"mul": func(a, b int) int { return a * b },
	}

	pages := []string{
		"directory",
		"hospital_detail",
		"specialization_detail",
		"doctor_detail",
		"browse_geo",
		"admin_login",
		"admin_dashboard",
		"admin_import_master",
		"admin_import_secondary",
		"admin_import_history",
		"admin_hospitals",
		"admin_doctors",
		"admin_custom_fields",
		"admin_settings",
	}

	layoutPath := filepath.Join("templates", "layout.html")

	for _, page := range pages {
		pagePath := filepath.Join("templates", page+".html")
		t, err := template.New("layout.html").Funcs(funcMap).ParseFiles(layoutPath, pagePath)
		if err != nil {
			return nil, fmt.Errorf("error parsing template %s: %w", page, err)
		}
		templates[page] = t
	}

	return templates, nil
}
