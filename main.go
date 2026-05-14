// art-gallery — a tiny single-binary website serving a fixed gallery of
// public-domain fine art works.
//
// Routes (all under the /art-gallery/ Gateway prefix):
//   GET /art-gallery/                       -> HTML grid of all 6 works
//   GET /art-gallery/works/{id}             -> HTML detail page for one work
//   GET /art-gallery/api/works              -> JSON array of all works
//   GET /art-gallery/api/works/{id}         -> JSON for one work or 404
//   GET /art-gallery/static/works/{slug}.svg -> SVG bytes from embed.FS
//   GET /art-gallery/healthz                -> "ok" (200) for probes
//
// Listens on :8080. Logs every request: method + path + status + microseconds.
// No external dependencies — std lib only. Six works hardcoded in the
// `works` slice; SVGs live in static/works/ and are embedded at build time.
package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"strings"
	"time"
)

// Work is one piece in the permanent collection. Same struct shape backs
// the JSON API and the HTML template.
type Work struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Artist string `json:"artist"`
	Year   string `json:"year"`   // string because some pieces are "c. 1665" or "1884–1886"
	Museum string `json:"museum"` // current location
	Slug   string `json:"slug"`   // lowercase, kebab-case, matches the SVG filename
}

// SVGPath returns the URL the gallery serves the work's image at.
// Used by the HTML template — kept here so the route prefix is in one place.
func (w Work) SVGPath() string {
	return "/art-gallery/static/works/" + w.Slug + ".svg"
}

// DetailPath returns the URL to this work's detail page.
func (w Work) DetailPath() string {
	return "/art-gallery/works/" + w.ID
}

// Permanent collection. Order in this slice = display order on the grid.
var works = []Work{
	{ID: "w1", Title: "The Starry Night", Artist: "Vincent van Gogh", Year: "1889", Museum: "Museum of Modern Art, New York", Slug: "starry-night"},
	{ID: "w2", Title: "The Great Wave off Kanagawa", Artist: "Katsushika Hokusai", Year: "c. 1831", Museum: "Multiple collections", Slug: "great-wave"},
	{ID: "w3", Title: "Mona Lisa", Artist: "Leonardo da Vinci", Year: "c. 1503–1519", Museum: "Louvre Museum, Paris", Slug: "mona-lisa"},
	{ID: "w4", Title: "Girl with a Pearl Earring", Artist: "Johannes Vermeer", Year: "c. 1665", Museum: "Mauritshuis, The Hague", Slug: "pearl-earring"},
	{ID: "w5", Title: "The Scream", Artist: "Edvard Munch", Year: "1893", Museum: "National Gallery of Norway", Slug: "the-scream"},
	{ID: "w6", Title: "A Sunday on La Grande Jatte", Artist: "Georges Seurat", Year: "1884–1886", Museum: "Art Institute of Chicago", Slug: "la-grande-jatte"},
}

//go:embed index.html
var templateFS embed.FS

//go:embed static/works/*.svg
var staticFS embed.FS

// pageTemplate is the single HTML template used by both list + detail pages.
// Parsed once at startup; if it doesn't compile the binary refuses to start.
var pageTemplate = template.Must(template.ParseFS(templateFS, "index.html"))

func main() {
	mux := http.NewServeMux()

	mux.HandleFunc("/art-gallery/", handleIndex)                    // catch-all under prefix; routes inside
	mux.HandleFunc("/art-gallery/healthz", handleHealth)
	mux.HandleFunc("/art-gallery/api/works", handleListJSON)
	mux.HandleFunc("/art-gallery/api/works/", handleGetJSON)        // trailing slash → match works/{id}
	mux.HandleFunc("/art-gallery/static/", handleStatic)
	// /art-gallery/works/{id} also handled inside handleIndex's switch

	addr := ":8080"
	log.Printf("art-gallery listening on %s", addr)
	if err := http.ListenAndServe(addr, logging(mux)); err != nil {
		log.Fatalf("server died: %v", err)
	}
}

// handleIndex serves the grid (at /art-gallery/) and the per-work detail
// pages (at /art-gallery/works/{id}). Anything unknown under /art-gallery/
// returns 404.
func handleIndex(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/art-gallery")
	switch {
	case path == "" || path == "/":
		render(w, "list", map[string]any{"Works": works})
	case strings.HasPrefix(path, "/works/"):
		id := strings.TrimPrefix(path, "/works/")
		one, ok := findByID(id)
		if !ok {
			http.NotFound(w, r)
			return
		}
		render(w, "detail", map[string]any{"Work": one})
	default:
		http.NotFound(w, r)
	}
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprint(w, "ok")
}

func handleListJSON(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(works); err != nil {
		log.Printf("encode list: %v", err)
	}
}

func handleGetJSON(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/art-gallery/api/works/")
	one, ok := findByID(id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(one); err != nil {
		log.Printf("encode one: %v", err)
	}
}

// handleStatic serves files from the embedded static/ directory. Uses
// http.FileServer with a sub-FS so paths line up.
func handleStatic(w http.ResponseWriter, r *http.Request) {
	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		http.Error(w, "static fs", http.StatusInternalServerError)
		return
	}
	// Set explicit content-type for SVGs so browsers render them inline.
	if strings.HasSuffix(r.URL.Path, ".svg") {
		w.Header().Set("Content-Type", "image/svg+xml")
		w.Header().Set("Cache-Control", "public, max-age=3600")
	}
	http.StripPrefix("/art-gallery/static/", http.FileServer(http.FS(sub))).ServeHTTP(w, r)
}

func findByID(id string) (Work, bool) {
	for _, w := range works {
		if w.ID == id {
			return w, true
		}
	}
	return Work{}, false
}

// render executes the named block from the template against the data and
// writes the result. On error logs + 500s.
func render(w http.ResponseWriter, block string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pageTemplate.ExecuteTemplate(w, block, data); err != nil {
		log.Printf("render %s: %v", block, err)
		http.Error(w, "render", http.StatusInternalServerError)
	}
}

// logging is a one-shot middleware that records method, path, status, and
// duration in microseconds. Wraps the response writer to capture the status.
func logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(ww, r)
		log.Printf("%s %s %d %s", r.Method, r.URL.Path, ww.status, time.Since(start))
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}
