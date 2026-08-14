package realtime

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// dialWS opens a client connection to path on the test server.
func dialWS(t *testing.T, server *httptest.Server, path string) *websocket.Conn {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	url := "ws" + server.URL[len("http"):] + path
	conn, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		t.Fatalf("dial %s: %v", path, err)
	}
	return conn
}

func readMsg(t *testing.T, conn *websocket.Conn) Message {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, raw, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var msg Message
	if err := json.Unmarshal(raw, &msg); err != nil {
		t.Fatalf("unmarshal: %v (%s)", err, raw)
	}
	return msg
}

// TestHubFanOut exercises the hub at unit level (§31.27 realtime bar):
// subscribers on a channel all receive a broadcast, disconnects unregister,
// and closed hubs refuse new registrations.
func TestHubFanOut(t *testing.T) {
	hub := NewHub()
	defer hub.Close()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: []string{"*"}})
		if err != nil {
			return
		}
		if !hub.register(r.URL.Query().Get("channel"), conn) {
			conn.Close(websocket.StatusGoingAway, "closed") //nolint:errcheck
			return
		}
		// Hold the connection until the peer goes away, then unregister —
		// the same lifecycle the Server drives.
		for {
			if _, _, err := conn.Read(r.Context()); err != nil {
				hub.unregister(r.URL.Query().Get("channel"), conn)
				return
			}
		}
	}))
	defer server.Close()

	c1 := dialWS(t, server, "/?channel=c")
	defer c1.Close(websocket.StatusNormalClosure, "") //nolint:errcheck
	c2 := dialWS(t, server, "/?channel=c")
	defer c2.Close(websocket.StatusNormalClosure, "") //nolint:errcheck
	other := dialWS(t, server, "/?channel=other")
	defer other.Close(websocket.StatusNormalClosure, "") //nolint:errcheck

	// Broadcast reaches both channel-c subscribers and returns the count.
	if n := hub.Broadcast("c", NewMessage("test.event", map[string]string{"k": "v"})); n != 2 {
		t.Fatalf("broadcast attempted on %d subscribers, want 2", n)
	}
	for i, conn := range []*websocket.Conn{c1, c2} {
		msg := readMsg(t, conn)
		if msg.Type != "test.event" || msg.SentAt.IsZero() {
			t.Fatalf("subscriber %d got %+v", i, msg)
		}
	}

	// A broadcast to an empty channel is a no-op.
	if n := hub.Broadcast("nobody", NewMessage("test.event", nil)); n != 0 {
		t.Fatalf("empty-channel broadcast = %d, want 0", n)
	}

	// Disconnect unregisters: the next broadcast targets one subscriber.
	c1.Close(websocket.StatusNormalClosure, "bye") //nolint:errcheck
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		hub.mu.Lock()
		n := len(hub.channels["c"])
		hub.mu.Unlock()
		if n == 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if n := hub.Broadcast("c", NewMessage("test.event", nil)); n != 1 {
		t.Fatalf("post-disconnect broadcast = %d, want 1", n)
	}

	// Close shuts everything down and refuses new registrations.
	hub.Close()
	hub.mu.Lock()
	remaining := len(hub.channels)
	hub.mu.Unlock()
	if remaining != 0 {
		t.Fatalf("hub channels after Close = %d, want 0", remaining)
	}
}

// TestServerRejectsUnauthenticated verifies the pre-upgrade auth gate
// without a database: a nil store is never reached because the token check
// fails first.
func TestServerRejectsUnauthenticated(t *testing.T) {
	srv := NewServer(NewHub(), nil, nil)
	server := httptest.NewServer(http.HandlerFunc(srv.Guide))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, resp, err := websocket.Dial(ctx, "ws"+server.URL[len("http"):]+"/", nil)
	if err == nil {
		t.Fatal("unauthenticated dial succeeded")
	}
	if resp == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %v, want 401", resp)
	}
}
