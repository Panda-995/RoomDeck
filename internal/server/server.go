package server

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schema string

type Config struct{ DataDir, WebDir, BaseURL, MediaURL, MediaKey, MediaSecret string }
type Server struct {
	db         *sql.DB
	lockFile   *os.File
	config     Config
	mu         sync.Mutex
	setupToken string
	hub        *Hub
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	limitMu    sync.Mutex
	limits     map[string]rateBucket
	mediaMu    sync.Mutex
	uploadMu   sync.Mutex
}
type rateBucket struct {
	count int
	reset time.Time
}
type principal struct{ Role, ID, Name, RoomID string }

func New(config Config) (*Server, error) {
	if config.DataDir == "" {
		config.DataDir = "data"
	}
	var err error
	config.DataDir, err = filepath.Abs(config.DataDir)
	if err != nil {
		return nil, err
	}
	if config.BaseURL != "" {
		u, e := url.Parse(config.BaseURL)
		if e != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
			return nil, errors.New("BASE_URL must be an HTTP(S) origin without a path")
		}
		config.BaseURL = strings.TrimRight(config.BaseURL, "/")
	}
	for _, d := range []string{"", "rooms", "tmp", "uploads"} {
		if err = os.MkdirAll(filepath.Join(config.DataDir, d), 0700); err != nil {
			return nil, err
		}
	}
	lockFile, err := lockData(filepath.Join(config.DataDir, ".roomdeck.lock"))
	if err != nil {
		return nil, errors.New("data directory is already in use or cannot be locked")
	}
	started := false
	defer func() {
		if !started {
			_ = lockFile.Close()
		}
	}()
	db, err := sql.Open("sqlite", filepath.Join(config.DataDir, "roomdeck.db"))
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if err = migrate(db); err != nil {
		db.Close()
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	s := &Server{db: db, lockFile: lockFile, config: config, hub: newHub(), ctx: ctx, cancel: cancel, limits: map[string]rateBucket{}}
	var count int
	if err = db.QueryRow("SELECT COUNT(*) FROM admins").Scan(&count); err != nil {
		db.Close()
		cancel()
		return nil, err
	}
	if count == 0 {
		tokenPath := filepath.Join(config.DataDir, "setup-token")
		b, e := os.ReadFile(tokenPath)
		if e == nil {
			s.setupToken = strings.TrimSpace(string(b))
		} else {
			s.setupToken = randomID(24)
			if e = os.WriteFile(tokenPath, []byte(s.setupToken), 0600); e != nil {
				db.Close()
				cancel()
				return nil, e
			}
		}
		log.Printf("First-run setup token: %s (also saved in %s)", s.setupToken, tokenPath)
	}
	// Durable upload reservations survive a process restart.
	if _, err = db.Exec("UPDATE rooms SET reserved=COALESCE((SELECT SUM(reservation) FROM uploads WHERE uploads.room_id=rooms.id AND state='uploading'),0); UPDATE jobs SET state='failed',error='SERVER_RESTARTED' WHERE state IN ('queued','running'); UPDATE screens SET expires=0; UPDATE media_clients SET expires=0;"); err != nil {
		s.Close()
		return nil, err
	}
	entries, _ := os.ReadDir(filepath.Join(config.DataDir, "tmp"))
	for _, e := range entries {
		_ = os.RemoveAll(filepath.Join(config.DataDir, "tmp", e.Name()))
	}
	if err = s.reconcileFiles(); err != nil {
		s.Close()
		return nil, err
	}
	s.wg.Add(1)
	go s.maintenance()
	s.wg.Add(1)
	go s.mediaMaintenance()
	s.wg.Add(1)
	go s.uploadMaintenance()
	_, _ = s.db.Exec("UPDATE screen_requests SET state='expired' WHERE state IN ('offered','presenting')")
	started = true
	return s, nil
}
func (s *Server) Close() {
	s.cancel()
	s.hub.close()
	s.wg.Wait()
	_ = s.db.Close()
	if s.lockFile != nil {
		_ = s.lockFile.Close()
	}
}
func (s *Server) Handler() http.Handler {
	m := http.NewServeMux()
	m.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, map[string]bool{"ok": true}) })
	m.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		if s.db.Ping() != nil {
			apiError(w, 503, "NOT_READY")
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	})
	m.HandleFunc("GET /api/v1/status", s.status)
	m.HandleFunc("POST /api/v1/setup", s.setup)
	m.HandleFunc("POST /api/v1/login", s.login)
	m.HandleFunc("POST /api/v1/logout", s.logout)
	m.HandleFunc("GET /api/v1/rooms", s.listRooms)
	m.HandleFunc("POST /api/v1/rooms", s.createRoom)
	m.HandleFunc("POST /api/v1/join", s.join)
	m.HandleFunc("POST /api/v1/invite-info", s.inviteInfo)
	m.HandleFunc("GET /api/v1/rooms/{room}/snapshot", s.snapshot)
	m.HandleFunc("PATCH /api/v1/rooms/{room}", s.updateRoom)
	m.HandleFunc("POST /api/v1/rooms/{room}/close", s.closeRoom)
	m.HandleFunc("DELETE /api/v1/rooms/{room}", s.deleteRoom)
	m.HandleFunc("POST /api/v1/rooms/{room}/invite", s.rotateInvite)
	m.HandleFunc("POST /api/v1/rooms/{room}/contents", s.createContent)
	m.HandleFunc("POST /api/v1/rooms/{room}/assets", s.uploadAsset)
	m.HandleFunc("POST /api/v1/rooms/{room}/uploads", s.createUpload)
	m.HandleFunc("GET /api/v1/rooms/{room}/uploads", s.listUploads)
	m.HandleFunc("GET /api/v1/rooms/{room}/uploads/{upload}", s.getUploadStatus)
	m.HandleFunc("DELETE /api/v1/rooms/{room}/uploads/{upload}", s.cancelUpload)
	m.HandleFunc("PUT /api/v1/rooms/{room}/uploads/{upload}/parts/{part}", s.uploadPart)
	m.HandleFunc("POST /api/v1/rooms/{room}/uploads/{upload}/complete", s.completeUpload)
	m.HandleFunc("PATCH /api/v1/rooms/{room}/contents/{content}", s.updateContent)
	m.HandleFunc("DELETE /api/v1/rooms/{room}/contents/{content}", s.deleteContent)
	m.HandleFunc("GET /api/v1/rooms/{room}/assets/{content}/{variant}", s.serveAsset)
	m.HandleFunc("PUT /api/v1/rooms/{room}/polls/{content}/vote", s.vote)
	m.HandleFunc("PATCH /api/v1/rooms/{room}/participants/{participant}", s.updateParticipant)
	m.HandleFunc("POST /api/v1/rooms/{room}/display-session", s.displaySession)
	m.HandleFunc("POST /api/v1/rooms/{room}/display/control", s.displayControl)
	m.HandleFunc("POST /api/v1/display-exchange", s.displayExchange)
	m.HandleFunc("POST /api/v1/rooms/{room}/exports", s.createExport)
	m.HandleFunc("GET /api/v1/rooms/{room}/exports/{job}", s.downloadExport)
	m.HandleFunc("GET /api/v1/rooms/{room}/ws", s.websocket)
	m.HandleFunc("GET /api/v1/rooms/{room}/storage", s.storageManifest)
	m.HandleFunc("GET /api/v1/rooms/{room}/interactions", s.interactions)
	m.HandleFunc("PUT /api/v1/rooms/{room}/contents/{content}/reaction", s.setReaction)
	m.HandleFunc("POST /api/v1/rooms/{room}/danmaku", s.sendDanmaku)
	m.HandleFunc("POST /api/v1/rooms/{room}/danmaku/moderate", s.moderateDanmaku)
	m.HandleFunc("PUT /api/v1/rooms/{room}/participants/{participant}/permissions", s.setCohost)
	m.HandleFunc("GET /api/v1/rooms/{room}/board", s.boardSnapshot)
	m.HandleFunc("POST /api/v1/rooms/{room}/board", s.boardAction)
	m.HandleFunc("GET /api/v1/rooms/{room}/game", s.gameSnapshot)
	m.HandleFunc("POST /api/v1/rooms/{room}/game", s.gameAction)
	m.HandleFunc("GET /api/v1/rooms/{room}/screen", s.screenSnapshot)
	m.HandleFunc("GET /api/v1/rooms/{room}/screen/queue", s.screenQueue)
	m.HandleFunc("POST /api/v1/rooms/{room}/screen/queue", s.screenQueueAction)
	m.HandleFunc("POST /api/v1/rooms/{room}/screen", s.screenAction)
	m.HandleFunc("/media/", s.mediaProxy)
	m.HandleFunc("/", s.frontend)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' blob: data:; media-src 'self' blob:; connect-src 'self'; worker-src 'self' blob:; object-src 'none'; base-uri 'self'; frame-ancestors 'none'; form-action 'self'")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
		}
		if r.Method != "GET" && r.Method != "HEAD" && r.Method != "OPTIONS" {
			if !s.sameOrigin(r) {
				apiError(w, 403, "ORIGIN_REJECTED")
				return
			}
			if r.Header.Get("X-RoomDeck-Request") != "1" {
				apiError(w, 403, "CSRF_REJECTED")
				return
			}
		}
		m.ServeHTTP(w, r)
	})
}
func (s *Server) sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, e := url.Parse(origin)
	if e != nil {
		return false
	}
	if s.config.BaseURL != "" {
		return origin == s.config.BaseURL
	}
	return u.Host == r.Host && (u.Scheme == "http" || u.Scheme == "https")
}
func (s *Server) frontend(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		apiError(w, 404, "NOT_FOUND")
		return
	}
	if r.Method != "GET" {
		http.NotFound(w, r)
		return
	}
	p := filepath.Join(s.config.WebDir, filepath.FromSlash(strings.TrimPrefix(filepath.Clean("/"+r.URL.Path), string(filepath.Separator))))
	if st, e := os.Stat(p); e == nil && !st.IsDir() {
		http.ServeFile(w, r, p)
		return
	}
	index := filepath.Join(s.config.WebDir, "index.html")
	if _, e := os.Stat(index); e != nil {
		http.Error(w, "Build the frontend first: cd web && npm ci && npm run build", 503)
		return
	}
	http.ServeFile(w, r, index)
}
func randomID(n int) string {
	b := make([]byte, n)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b)
}
func digest(v string) string { b := sha256.Sum256([]byte(v)); return hex.EncodeToString(b[:]) }
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func apiError(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, map[string]interface{}{"error": map[string]string{"code": code}})
}
func readJSON(w http.ResponseWriter, r *http.Request, v interface{}) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if e := d.Decode(v); e != nil {
		apiError(w, 400, "INVALID_INPUT")
		return false
	}
	var extra interface{}
	if e := d.Decode(&extra); e != io.EOF {
		apiError(w, 400, "INVALID_INPUT")
		return false
	}
	return true
}
func validText(v string, min, max int) bool {
	return utf8.ValidString(v) && utf8.RuneCountInString(strings.TrimSpace(v)) >= min && utf8.RuneCountInString(v) <= max && !strings.ContainsRune(v, 0)
}
func (s *Server) internal(w http.ResponseWriter, e error) {
	log.Printf("operation failed: %v", e)
	apiError(w, 500, "INTERNAL_ERROR")
}
func (s *Server) limited(r *http.Request, key string, max int) bool {
	s.limitMu.Lock()
	defer s.limitMu.Unlock()
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	now := time.Now()
	if len(s.limits) > 10000 {
		for k, b := range s.limits {
			if now.After(b.reset) {
				delete(s.limits, k)
			}
		}
	}
	key = host + ":" + key
	b := s.limits[key]
	if now.After(b.reset) {
		b = rateBucket{reset: now.Add(time.Minute)}
	}
	b.count++
	s.limits[key] = b
	return b.count > max
}
func (s *Server) audit(room, action, actor string) {
	_, e := s.db.Exec("INSERT INTO audit VALUES(?,?,?,?,?)", randomID(16), room, action, actor, time.Now().Unix())
	if e != nil {
		log.Printf("audit write: %v", e)
	}
}
