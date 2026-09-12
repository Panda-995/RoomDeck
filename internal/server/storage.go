package server

import (
	"net/http"
	"path/filepath"
)

// The database remains the authority. This manifest exposes relative paths only to hosts.
func (s *Server) storageManifest(w http.ResponseWriter, r *http.Request) {
	p, room := s.roomAccess(w, r)
	if p == nil {
		return
	}
	if p.Role != "host" {
		apiError(w, 403, "FORBIDDEN")
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query("SELECT id,kind,filename,bytes FROM contents WHERE room_id=? AND kind IN ('photo','file') ORDER BY created_at", room.ID)
	if err != nil {
		s.internal(w, err)
		return
	}
	defer rows.Close()
	type item struct {
		ID       string            `json:"id"`
		Kind     string            `json:"kind"`
		Filename string            `json:"filename"`
		Bytes    int64             `json:"bytes"`
		Paths    map[string]string `json:"paths"`
	}
	items := []item{}
	for rows.Next() {
		var v item
		if err = rows.Scan(&v.ID, &v.Kind, &v.Filename, &v.Bytes); err != nil {
			s.internal(w, err)
			return
		}
		v.Paths = map[string]string{}
		variants := []string{"original"}
		if v.Kind == "photo" {
			variants = append(variants, "preview", "thumb")
		}
		for _, variant := range variants {
			v.Paths[variant] = filepath.ToSlash(filepath.Join("rooms", room.ID, v.ID+"."+variant))
		}
		items = append(items, v)
	}
	if err = rows.Err(); err != nil {
		s.internal(w, err)
		return
	}
	rows.Close()
	exports := []map[string]interface{}{}
	jobs, err := s.db.Query("SELECT id,bytes FROM jobs WHERE room_id=? AND state='ready'", room.ID)
	if err != nil {
		s.internal(w, err)
		return
	}
	defer jobs.Close()
	for jobs.Next() {
		var id string
		var size int64
		if err = jobs.Scan(&id, &size); err != nil {
			s.internal(w, err)
			return
		}
		exports = append(exports, map[string]interface{}{"id": id, "bytes": size, "path": filepath.ToSlash(filepath.Join("rooms", room.ID, "export-"+id+".zip"))})
	}
	if err = jobs.Err(); err != nil {
		s.internal(w, err)
		return
	}
	w.Header().Set("Content-Disposition", `attachment; filename="roomdeck-storage.json"`)
	writeJSON(w, 200, map[string]interface{}{"room_id": room.ID, "room_name": room.Name, "root": "/data", "files": items, "exports": exports, "metadata": "roomdeck.db"})
}
