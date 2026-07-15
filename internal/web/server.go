package web

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"opertraitor/internal/models"
	"opertraitor/internal/storage"
	"opertraitor/internal/ui"
	"opertraitor/internal/utils"
	"sort"
	"time"
)

type Server struct {
	store   storage.Store
	address string
	port    string
}

func NewServer(store storage.Store, address, port string) *Server {
	return &Server{
		store:   store,
		address: address,
		port:    port,
	}
}

func (s *Server) Start() {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleRequest)

	fullAddr := fmt.Sprintf("%s:%s", s.address, s.port)
	ui.PrintHeader("Web UI Started")

	if s.address == "0.0.0.0" {
		ui.LogWarn(fmt.Sprintf("Warning: Server is listening on all interfaces (%s). Ensure this is intended.", fullAddr))
	} else {
		ui.LogInfo(fmt.Sprintf("Server is bound to %s (Localhost only)", s.address))
	}

	ui.LogSuccess(fmt.Sprintf("Dashboard available at http://%s:%s", s.address, s.port))

	// Configure explicit timeouts to mitigate Slowloris and slow-POST attacks.
	srv := &http.Server{
		Addr:              fullAddr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20, // 1 MiB
	}

	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

// PageData holds UI elements
type PageData struct {
	IsDetail      bool
	Items         []models.OperatorRbacInfo
	Item          models.OperatorRbacInfo
	Query         string
	CurrentCat    string   // Selected category
	AllCategories []string // List for sidebar
	Stats         Stats
}

type Stats struct {
	Total    int
	Analyzed int
	HighRisk int
}

func (s *Server) handleRequest(w http.ResponseWriter, r *http.Request) {
	// Sanitize query params.
	name := sanitizeInput(r.URL.Query().Get("name"), 200)
	version := sanitizeInput(r.URL.Query().Get("version"), 100)
	itemID := sanitizeInput(r.URL.Query().Get("id"), 200)
	catFilter := sanitizeInput(r.URL.Query().Get("cat"), 100)
	query := sanitizeInput(r.URL.Query().Get("q"), 200)

	data := PageData{
		Query:      query,
		CurrentCat: catFilter,
	}

	if itemID != "" || name != "" {
		// --- DETAIL MODE ---
		data.IsDetail = true
		if itemID != "" {
			allItems, err := s.store.Scan(r.Context(), models.FilterOptions{})
			if err != nil {
				http.Error(w, "Internal server error", 500)
				return
			}
			for _, item := range allItems {
				if item.ID == itemID {
					data.Item = item
					break
				}
			}
			if data.Item.ID == "" {
				http.Error(w, "Operator not found", 404)
				return
			}
		} else {
			item, err := s.store.Get(name, version)
			if err != nil {
				// Sanitize error message
				http.Error(w, "Operator not found", 404)
				return
			}
			data.Item = item
		}
	} else {
		// --- DASHBOARD MODE ---
		data.IsDetail = false

		//Fetch EVERYTHING first (Empty Filter)
		// This bypasses the strict storage filter and lets us do fuzzy matching.
		emptyFilter := models.FilterOptions{}
		allItems, err := s.store.Scan(r.Context(), emptyFilter)
		if err != nil {
			// Sanitize error message to prevent information leakage
			http.Error(w, "Internal server error", 500)
			return
		}

		// Prepare the Search Filter
		searchFilter := models.FilterOptions{OperatorName: data.Query}

		// Build Category List (From all items) & Filter Items in Memory
		catMap := make(map[string]bool)
		var filteredItems []models.OperatorRbacInfo

		for _, item := range allItems {
			// Collect Categories for Sidebar
			for _, c := range item.Metadata.Categories {
				catMap[c] = true
			}

			// Apply Search Filter (Fuzzy Match via Match())
			if !searchFilter.Match(item.OperatorName, item.Version) {
				continue
			}

			// Apply Category Filter (Exact Match)
			if catFilter != "" {
				inCategory := false
				for _, c := range item.Metadata.Categories {
					if c == catFilter {
						inCategory = true
						break
					}
				}
				if !inCategory {
					continue
				}
			}

			// If passed both filters, add to display list
			filteredItems = append(filteredItems, item)
		}

		// Sort Categories for sidebar
		for c := range catMap {
			data.AllCategories = append(data.AllCategories, c)
		}
		sort.Strings(data.AllCategories)

		data.Items = filteredItems

		// Calculate Stats (Based on the filtered view)
		data.Stats.Total = len(data.Items)
		for _, i := range data.Items {
			if i.Analysis != nil {
				data.Stats.Analyzed++
				if i.Analysis.OverallRiskScore >= 9 {
					data.Stats.HighRisk++
				}
			}
		}
	}

	s.render(w, data)
}

func (s *Server) render(w http.ResponseWriter, data PageData) {
	// Security headers
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
	// CSP: the template embeds inline <style> blocks but no inline <script>,
	// so we keep 'unsafe-inline' for styles only and forbid it for scripts.
	w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; script-src 'none'; img-src 'self' data:; base-uri 'none'; form-action 'self'; frame-ancestors 'none'")

	funcMap := template.FuncMap{
		"riskColor": func(score int) string {
			if score <= 3 {
				return "#10b981"
			}
			if score <= 8 {
				return "#f59e0b"
			}
			return "#ef4444"
		},
		"markdown": utils.RenderSafeHTML,
		"isOld": func(t time.Time) bool {
			if t.IsZero() {
				return false
			}
			cutoff := time.Now().AddDate(-3, 0, 0)
			return t.Before(cutoff)
		},
		"formatDate": func(t time.Time) string {
			if t.IsZero() {
				return "Unknown"
			}
			return t.Format("Mon Jan 02, 2006")
		},
	}

	tmpl, err := template.New("index").Funcs(funcMap).Parse(FullHtmlTemplate)
	if err != nil {
		// Log detailed error server-side but return generic message to client
		log.Printf("Template error: %v", err)
		http.Error(w, "Internal server error", 500)
		return
	}

	if err := tmpl.Execute(w, data); err != nil {
		log.Printf("Render error: %v", err)
	}
}

// sanitizeInput caps length and strips control chars.
func sanitizeInput(input string, maxLength int) string {
	if len(input) > maxLength {
		input = input[:maxLength]
	}
	// Remove null bytes and control characters
	cleaned := make([]rune, 0, len(input))
	for _, r := range input {
		if r >= 32 && r != 127 { // Allow printable ASCII except DEL
			cleaned = append(cleaned, r)
		}
	}
	return string(cleaned)
}
