package server

import (
	"os"
	"path/filepath"
	"strings"
)

// Files are named only by server-generated IDs. Reconcile interrupted filesystem
// writes at startup while the exclusive data-directory lock is held.
func (s *Server) reconcileFiles() error {
	rows, e := s.db.Query("SELECT id FROM rooms")
	if e != nil {
		return e
	}
	rooms := []string{}
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			rows.Close()
			return e
		}
		rooms = append(rooms, id)
	}
	rows.Close()
	for _, room := range rooms {
		keep := map[string]bool{}
		rows, e = s.db.Query("SELECT id,kind FROM contents WHERE room_id=?", room)
		if e != nil {
			return e
		}
		for rows.Next() {
			var id, kind string
			if e = rows.Scan(&id, &kind); e != nil {
				rows.Close()
				return e
			}
			if kind == "photo" || kind == "file" {
				keep[id+".original"] = true
			}
			if kind == "photo" {
				keep[id+".preview"] = true
				keep[id+".thumb"] = true
			}
		}
		rows.Close()
		rows, e = s.db.Query("SELECT id FROM jobs WHERE room_id=? AND state='ready'", room)
		if e != nil {
			return e
		}
		for rows.Next() {
			var id string
			if e = rows.Scan(&id); e != nil {
				rows.Close()
				return e
			}
			keep["export-"+id+".zip"] = true
		}
		rows.Close()
		dir := filepath.Join(s.config.DataDir, "rooms", room)
		files, e := os.ReadDir(dir)
		if os.IsNotExist(e) {
			continue
		}
		if e != nil {
			return e
		}
		for _, file := range files {
			if file.IsDir() {
				continue
			}
			name := file.Name()
			owned := strings.HasSuffix(name, ".original") || strings.HasSuffix(name, ".preview") || strings.HasSuffix(name, ".thumb") || strings.HasPrefix(name, "export-")
			if owned && !keep[name] {
				if e = os.Remove(filepath.Join(dir, name)); e != nil {
					return e
				}
			}
		}
	}
	return nil
}
