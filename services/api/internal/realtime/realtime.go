// Package realtime implements the WebSocket layer (spec §11): a small hub
// with per-channel fan-out and the three WS endpoints — /ws/guide,
// /ws/booking/{id} and /ws/admin/operations (spec §13.5).
//
// Realtime is a convenience channel, never the source of truth: every piece
// of data pushed here is also readable via REST (GET /me/guide/offers, GET
// /bookings/{id}, GET /bookings/{id}/location), so clients reconnect and
// catch up through REST after any disconnect (spec §31.27).
//
// The library is github.com/coder/websocket: context-native I/O (write
// timeouts and shutdown compose with request contexts) and actively
// maintained, unlike the archived gorilla/websocket.
package realtime

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"

	pauth "proguidegh/api/internal/platform/auth"
	"proguidegh/api/internal/platform/httpx"
	"proguidegh/api/internal/platform/rbac"
)

// Channels used across the platform.
const (
	// ChannelAdminOperations is the operations feed (all active tours).
	ChannelAdminOperations = "admin:operations"
)

// ChannelGuide is one guide's offer feed.
func ChannelGuide(guideID string) string { return "guide:" + guideID }

// ChannelBooking is one booking's tourist/ops feed.
func ChannelBooking(bookingID string) string { return "booking:" + bookingID }

// Message is the single JSON envelope pushed on every channel.
type Message struct {
	Type   string    `json:"type"`
	Data   any       `json:"data"`
	SentAt time.Time `json:"sent_at"`
}

// NewMessage stamps an envelope.
func NewMessage(msgType string, data any) Message {
	return Message{Type: msgType, Data: data, SentAt: time.Now().UTC()}
}

const (
	// writeTimeout bounds one fan-out write; a slow consumer is dropped
	// rather than blocking the publisher (ADR 0003: realtime is ephemeral).
	writeTimeout = 5 * time.Second
	// pingInterval is the server keepalive cadence; a missed pong closes the
	// connection so dead peers are reaped.
	pingInterval = 30 * time.Second
)

// Hub fans messages out to subscribed connections by channel. Register/
// unregister are driven by the Server's connection lifecycle; Broadcast is
// called by domain services after their database commit.
type Hub struct {
	mu       sync.Mutex
	channels map[string]map[*websocket.Conn]struct{}
	closed   bool
}

// NewHub builds an empty hub.
func NewHub() *Hub {
	return &Hub{channels: map[string]map[*websocket.Conn]struct{}{}}
}

// Broadcast sends msg to every connection subscribed to channel and returns
// the number of subscribers the write was attempted on. Failed writes
// unregister the connection immediately.
func (h *Hub) Broadcast(channel string, msg Message) int {
	raw, err := json.Marshal(msg)
	if err != nil {
		slog.Error("realtime: marshal broadcast", "type", msg.Type, "error", err)
		return 0
	}

	h.mu.Lock()
	subs := make([]*websocket.Conn, 0, len(h.channels[channel]))
	for c := range h.channels[channel] {
		subs = append(subs, c)
	}
	h.mu.Unlock()

	for _, c := range subs {
		ctx, cancel := context.WithTimeout(context.Background(), writeTimeout)
		err := c.Write(ctx, websocket.MessageText, raw)
		cancel()
		if err != nil {
			h.unregister(channel, c)
			c.Close(websocket.StatusGoingAway, "write failed") //nolint:errcheck
		}
	}
	return len(subs)
}

func (h *Hub) register(channel string, c *websocket.Conn) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return false
	}
	if h.channels[channel] == nil {
		h.channels[channel] = map[*websocket.Conn]struct{}{}
	}
	h.channels[channel][c] = struct{}{}
	return true
}

func (h *Hub) unregister(channel string, c *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.channels[channel], c)
	if len(h.channels[channel]) == 0 {
		delete(h.channels, channel)
	}
}

// Close shuts every connection down gracefully (StatusGoingAway) and refuses
// new registrations. Called on server shutdown.
func (h *Hub) Close() {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return
	}
	h.closed = true
	conns := []*websocket.Conn{}
	for _, subs := range h.channels {
		for c := range subs {
			conns = append(conns, c)
		}
	}
	h.channels = map[string]map[*websocket.Conn]struct{}{}
	h.mu.Unlock()

	for _, c := range conns {
		c.Close(websocket.StatusGoingAway, "server shutting down") //nolint:errcheck
	}
}

// Server serves the WS endpoints. Authentication is an access token in the
// ?token= query parameter (browser WS clients cannot set headers) or the
// session cookie/Bearer header; the permission set is reloaded live, same as
// REST (spec §3).
type Server struct {
	hub    *Hub
	issuer *pauth.TokenIssuer
	store  *rbac.Store

	// BookingVisible reports whether the identity may watch a booking
	// channel (owner tourist, assigned guide or bookings.read) — injected to
	// keep realtime free of booking persistence.
	BookingVisible func(ctx context.Context, bookingID string, id rbac.Identity) bool
	// GuideSnapshot returns the catch-up messages pushed on connect to a
	// guide channel (current unexpired offers). Optional.
	GuideSnapshot func(ctx context.Context, userID string) []Message
	// BookingSnapshot returns the catch-up messages pushed on connect to a
	// booking channel (current status, last known position). Optional.
	BookingSnapshot func(ctx context.Context, bookingID string) []Message
}

// NewServer builds the WS server.
func NewServer(hub *Hub, issuer *pauth.TokenIssuer, store *rbac.Store) *Server {
	return &Server{hub: hub, issuer: issuer, store: store}
}

// authenticate verifies the access token from ?token=, cookie or Bearer and
// loads the live permission set. On failure it writes the standard error
// envelope (the upgrade never happens).
func (s *Server) authenticate(w http.ResponseWriter, r *http.Request) (rbac.Identity, bool) {
	token := r.URL.Query().Get("token")
	if token == "" {
		token = pauth.AccessFromRequest(r)
	}
	if token == "" {
		httpx.WriteError(w, r, http.StatusUnauthorized, "UNAUTHENTICATED", "authentication required", nil)
		return rbac.Identity{}, false
	}
	claims, err := s.issuer.Verify(token)
	if err != nil {
		code := "UNAUTHENTICATED"
		if errors.Is(err, pauth.ErrTokenExpired) {
			code = "TOKEN_EXPIRED"
		}
		httpx.WriteError(w, r, http.StatusUnauthorized, code, "invalid or expired access token", nil)
		return rbac.Identity{}, false
	}
	// Same check as RequireAuth: a suspended or deleted account must not be
	// able to open a live channel with a still-unexpired access token.
	active, err := s.store.AccountActive(r.Context(), claims.Subject)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not verify account", nil)
		return rbac.Identity{}, false
	}
	if !active {
		httpx.WriteError(w, r, http.StatusUnauthorized, "ACCOUNT_INACTIVE", "this account is no longer active", nil)
		return rbac.Identity{}, false
	}
	perms, err := s.store.Permissions(r.Context(), claims.Subject)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not load permissions", nil)
		return rbac.Identity{}, false
	}
	id := rbac.Identity{UserID: claims.Subject, SessionID: claims.SessionID, Perms: map[string]struct{}{}}
	for _, p := range perms {
		id.Perms[p] = struct{}{}
	}
	return id, true
}

// Guide handles GET /ws/guide — the caller's own dispatch offer feed.
// Any authenticated user may connect; only their own offers ever arrive.
func (s *Server) Guide(w http.ResponseWriter, r *http.Request) {
	id, ok := s.authenticate(w, r)
	if !ok {
		return
	}
	var snapshot []Message
	if s.GuideSnapshot != nil {
		snapshot = s.GuideSnapshot(r.Context(), id.UserID)
	}
	s.serve(w, r, ChannelGuide(id.UserID), snapshot)
}

// Booking handles GET /ws/booking/{id} — status events and guide position
// for one booking. Visible to the owning tourist, the assigned guide and
// bookings.read holders; anyone else gets 403 (REST uses 404, but the WS
// path already revealed the id in the URL the caller supplied).
func (s *Server) Booking(w http.ResponseWriter, r *http.Request) {
	id, ok := s.authenticate(w, r)
	if !ok {
		return
	}
	bookingID := r.PathValue("id")
	if s.BookingVisible == nil || !s.BookingVisible(r.Context(), bookingID, id) {
		httpx.WriteError(w, r, http.StatusForbidden, "FORBIDDEN", "booking not visible to the caller", nil)
		return
	}
	var snapshot []Message
	if s.BookingSnapshot != nil {
		snapshot = s.BookingSnapshot(r.Context(), bookingID)
	}
	s.serve(w, r, ChannelBooking(bookingID), snapshot)
}

// AdminOperations handles GET /ws/admin/operations — the operations feed for
// all active tours (spec §11.2: requires the operations permission).
func (s *Server) AdminOperations(w http.ResponseWriter, r *http.Request) {
	id, ok := s.authenticate(w, r)
	if !ok {
		return
	}
	if !id.Has("dispatch.manage") {
		httpx.WriteError(w, r, http.StatusForbidden, "FORBIDDEN", "missing required permission",
			map[string]string{"permission": "dispatch.manage"})
		return
	}
	s.serve(w, r, ChannelAdminOperations, nil)
}

// serve upgrades the request, registers the connection on the channel,
// pushes the connect snapshot and then blocks on the read loop until the
// peer disconnects. Origin policy: token-authenticated API clients are the
// V1 consumers, so any origin is accepted; the SameSite=Lax session cookie
// is not sent on cross-site WS handshakes, and the ?token= credential is
// never ambient.
func (s *Server) serve(w http.ResponseWriter, r *http.Request, channel string, snapshot []Message) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: []string{"*"}})
	if err != nil {
		return // Accept already wrote the failure response
	}
	if !s.hub.register(channel, conn) {
		conn.Close(websocket.StatusGoingAway, "server shutting down") //nolint:errcheck
		return
	}
	defer func() {
		s.hub.unregister(channel, conn)
		conn.Close(websocket.StatusNormalClosure, "closing") //nolint:errcheck
	}()

	for _, msg := range snapshot {
		ctx, cancel := context.WithTimeout(r.Context(), writeTimeout)
		raw, _ := json.Marshal(msg)
		err := conn.Write(ctx, websocket.MessageText, raw)
		cancel()
		if err != nil {
			return
		}
	}

	// Keepalive: coder/websocket answers client pings automatically while we
	// read; our own pings reap dead peers.
	pingDone := make(chan struct{})
	go func() {
		ticker := time.NewTicker(pingInterval)
		defer ticker.Stop()
		for {
			select {
			case <-pingDone:
				return
			case <-ticker.C:
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				err := conn.Ping(ctx)
				cancel()
				if err != nil {
					conn.Close(websocket.StatusGoingAway, "ping failed") //nolint:errcheck
					return
				}
			}
		}
	}()
	defer close(pingDone)

	// Clients never send application messages on these channels; the read
	// loop exists to drive control frames and detect closure.
	for {
		if _, _, err := conn.Read(r.Context()); err != nil {
			return
		}
	}
}
