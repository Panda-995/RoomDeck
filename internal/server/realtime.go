package server

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
)

type subscriber struct {
	room, role  string
	ch          chan struct{}
	interaction chan struct{}
}
type Hub struct {
	mu   sync.Mutex
	subs map[*subscriber]bool
}

func newHub() *Hub { return &Hub{subs: map[*subscriber]bool{}} }
func (h *Hub) add(room, role string) *subscriber {
	h.mu.Lock()
	defer h.mu.Unlock()
	sub := &subscriber{room: room, role: role, ch: make(chan struct{}, 1), interaction: make(chan struct{}, 1)}
	h.subs[sub] = true
	return sub
}
func (h *Hub) remove(sub *subscriber) { h.mu.Lock(); defer h.mu.Unlock(); delete(h.subs, sub) }
func (h *Hub) notify(room string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for sub := range h.subs {
		if sub.room == room {
			select {
			case sub.ch <- struct{}{}:
			default:
			}
		}
	}
}
func (h *Hub) displayCount(room string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	n := 0
	for sub := range h.subs {
		if sub.room == room && sub.role == "display" {
			n++
		}
	}
	return n
}
func (h *Hub) notifyInteractions(room string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for sub := range h.subs {
		if sub.room == room {
			select {
			case sub.interaction <- struct{}{}:
			default:
			}
		}
	}
}
func (h *Hub) close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for sub := range h.subs {
		select {
		case sub.ch <- struct{}{}:
		default:
		}
	}
}
func (s *Server) websocket(w http.ResponseWriter, r *http.Request) {
	p, room := s.roomAccess(w, r)
	if p == nil {
		return
	}
	if !s.sameOrigin(r) {
		apiError(w, 403, "ORIGIN_REJECTED")
		return
	}
	opts := &websocket.AcceptOptions{}
	conn, e := websocket.Accept(w, r, opts)
	if e != nil {
		return
	}
	defer conn.CloseNow()
	conn.SetReadLimit(1024)
	ctx := conn.CloseRead(r.Context())
	sub := s.hub.add(room.ID, p.Role)
	defer s.hub.remove(sub)
	s.hub.notify(room.ID)
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()
	for {
		message := `{"type":"refresh"}`
		select {
		case <-ctx.Done():
			return
		case <-s.ctx.Done():
			return
		case <-ticker.C:
		case <-sub.ch:
		case <-sub.interaction:
			message = `{"type":"interactions"}`
		}
		current := s.identity(r, p.Role)
		fresh, e := s.getRoom(room.ID)
		if current == nil || e != nil || fresh.Expired() || (p.Role == "display" && !fresh.Enabled("display")) {
			_ = conn.Close(websocket.StatusPolicyViolation, "session expired")
			return
		}
		if p.Role == "guest" {
			_, _ = s.db.Exec("UPDATE participants SET last_seen=? WHERE id=?", time.Now().Unix(), p.ID)
		}
		send, cancel := context.WithTimeout(ctx, 5*time.Second)
		e = conn.Write(send, websocket.MessageText, []byte(message))
		cancel()
		if e != nil {
			return
		}
	}
}
