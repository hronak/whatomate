package websocket

import (
	"encoding/json"
	"sync"

	"github.com/google/uuid"
	"github.com/zerodha/logf"
)

// Hub maintains the set of active clients and broadcasts messages to them
type Hub struct {
	// clients maps organization ID -> user ID -> set of clients (supports multiple tabs)
	clients map[uuid.UUID]map[uuid.UUID]map[*Client]struct{}

	// broadcast channel for messages
	broadcast chan BroadcastMessage

	// register channel for new clients
	register chan *Client

	// unregister channel for disconnecting clients
	unregister chan *Client

	// quit signals Run to exit; closed once by Stop
	quit chan struct{}

	// stopOnce keeps Stop idempotent
	stopOnce sync.Once

	// mutex for thread-safe access to clients map
	mu sync.RWMutex

	// logger
	log logf.Logger
}

// NewHub creates a new Hub instance
func NewHub(log logf.Logger) *Hub {
	return &Hub{
		clients:    make(map[uuid.UUID]map[uuid.UUID]map[*Client]struct{}),
		broadcast:  make(chan BroadcastMessage, 256),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		quit:       make(chan struct{}),
		log:        log,
	}
}

// Run starts the hub's main loop. It returns when Stop is called.
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.registerClient(client)

		case client := <-h.unregister:
			h.unregisterClient(client)

		case message := <-h.broadcast:
			h.broadcastMessage(message)

		case <-h.quit:
			h.closeAllClients()
			return
		}
	}
}

// Stop shuts the hub down: Run returns and every connected client's send
// channel is closed, which unblocks its WritePump. Safe to call more than once
// and safe to call when Run was never started.
func (h *Hub) Stop() {
	h.stopOnce.Do(func() { close(h.quit) })
}

// closeAllClients drops every registered client. Closing send is what
// terminates each client's WritePump; ReadPump ends when its connection does.
func (h *Hub) closeAllClients() {
	h.mu.Lock()
	defer h.mu.Unlock()

	count := 0
	for orgID, orgClients := range h.clients {
		for userID, userClients := range orgClients {
			for client := range userClients {
				close(client.send)
				count++
			}
			delete(orgClients, userID)
		}
		delete(h.clients, orgID)
	}

	h.log.Info("WebSocket hub stopped", "clients_closed", count)
}

// registerClient adds a client to the hub
func (h *Hub) registerClient(client *Client) {
	h.mu.Lock()

	orgClients, ok := h.clients[client.OrgID()]
	if !ok {
		orgClients = make(map[uuid.UUID]map[*Client]struct{})
		h.clients[client.OrgID()] = orgClients
	}

	userClients, ok := orgClients[client.UserID()]
	if !ok {
		userClients = make(map[*Client]struct{})
		orgClients[client.UserID()] = userClients
	}

	// Add this client to the set (allows multiple tabs)
	userClients[client] = struct{}{}

	// Snapshot the counts, then log outside the write lock: the full walk in
	// countClients is O(clients) and blocked every broadcast while it ran.
	userConns, total := len(userClients), h.countClients()
	h.mu.Unlock()

	h.log.Info("WebSocket client registered",
		"user_id", client.UserID(),
		"org_id", client.OrgID(),
		"user_connections", userConns,
		"total_clients", total)
}

// unregisterClient removes a client from the hub
func (h *Hub) unregisterClient(client *Client) {
	h.mu.Lock()

	if orgClients, ok := h.clients[client.OrgID()]; ok {
		if userClients, ok := orgClients[client.UserID()]; ok {
			if _, exists := userClients[client]; exists {
				delete(userClients, client)
				close(client.send)

				// Clean up empty user map
				if len(userClients) == 0 {
					delete(orgClients, client.UserID())
				}

				// Clean up empty org map
				if len(orgClients) == 0 {
					delete(h.clients, client.OrgID())
				}
			}
		}
	}

	total := h.countClients()
	h.mu.Unlock()

	h.log.Info("WebSocket client unregistered",
		"user_id", client.UserID(),
		"org_id", client.OrgID(),
		"total_clients", total)
}

// maxSendDrops is how many consecutive full-buffer drops a client may incur
// before the hub gives up on it. A client that cannot keep up is not merely
// missing one event — its view is already inconsistent, and silently dropping
// more leaves it wrong indefinitely. Disconnecting forces a reconnect and a
// fresh fetch.
const maxSendDrops = 32

// broadcastMessage sends a message to all relevant clients.
func (h *Hub) broadcastMessage(msg BroadcastMessage) {
	data, err := json.Marshal(msg.Message)
	if err != nil {
		h.log.Error("Failed to marshal broadcast message", "error", err)
		return
	}

	h.mu.RLock()
	orgClients, ok := h.clients[msg.OrgID]
	if !ok {
		h.mu.RUnlock()
		return
	}

	var stalled []*Client
	if msg.UserID != uuid.Nil {
		// Only send to that user's clients
		for client := range orgClients[msg.UserID] {
			if !h.deliver(client, data) {
				stalled = append(stalled, client)
			}
		}
	} else {
		for _, userClients := range orgClients {
			for client := range userClients {
				// If ContactID is specified, only send to clients viewing that contact
				if msg.ContactID != uuid.Nil && client.viewingOtherContact(msg.ContactID) {
					continue
				}
				if !h.deliver(client, data) {
					stalled = append(stalled, client)
				}
			}
		}
	}
	h.mu.RUnlock()

	// Drop hopeless clients after releasing the read lock — unregisterClient
	// takes the write lock.
	for _, client := range stalled {
		h.log.Warn("Dropping unresponsive WebSocket client",
			"user_id", client.UserID(),
			"org_id", client.OrgID(),
			"consecutive_drops", maxSendDrops)
		h.unregisterClient(client)
		client.closeConn()
	}
}

// deliver queues data for one client, reporting false when the client has
// stalled past maxSendDrops and should be disconnected.
func (h *Hub) deliver(client *Client, data []byte) bool {
	select {
	case client.send <- data:
		client.resetDrops()
		return true
	default:
		drops := client.recordDrop()
		h.log.Warn("Client send buffer full, skipping",
			"user_id", client.UserID(),
			"org_id", client.OrgID(),
			"consecutive_drops", drops)
		return drops < maxSendDrops
	}
}

// Broadcast sends a message to the broadcast channel
func (h *Hub) Broadcast(msg BroadcastMessage) {
	select {
	case h.broadcast <- msg:
	default:
		h.log.Warn("Broadcast channel full, dropping message")
	}
}

// BroadcastToOrg sends a message to all clients in an organization
func (h *Hub) BroadcastToOrg(orgID uuid.UUID, msg WSMessage) {
	h.Broadcast(BroadcastMessage{
		OrgID:   orgID,
		Message: msg,
	})
}

// BroadcastToContact sends a message to clients viewing a specific contact
func (h *Hub) BroadcastToContact(orgID, contactID uuid.UUID, msg WSMessage) {
	h.Broadcast(BroadcastMessage{
		OrgID:     orgID,
		ContactID: contactID,
		Message:   msg,
	})
}

// BroadcastToUser sends a message to a specific user
func (h *Hub) BroadcastToUser(orgID, userID uuid.UUID, msg WSMessage) {
	h.Broadcast(BroadcastMessage{
		OrgID:   orgID,
		UserID:  userID,
		Message: msg,
	})
}

// BroadcastToUsers sends a message to multiple users
func (h *Hub) BroadcastToUsers(orgID uuid.UUID, userIDs []uuid.UUID, msg WSMessage) {
	for _, userID := range userIDs {
		h.BroadcastToUser(orgID, userID, msg)
	}
}

// countClients returns the total number of connected clients
func (h *Hub) countClients() int {
	count := 0
	for _, orgClients := range h.clients {
		for _, userClients := range orgClients {
			count += len(userClients)
		}
	}
	return count
}

// GetClientCount returns the number of connected clients (thread-safe)
func (h *Hub) GetClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.countClients()
}

// IsUserOnline returns true if the user has at least one active WebSocket connection.
func (h *Hub) IsUserOnline(orgID, userID uuid.UUID) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if orgClients, ok := h.clients[orgID]; ok {
		if userClients, ok := orgClients[userID]; ok {
			return len(userClients) > 0
		}
	}
	return false
}

// OnlineUserIDs returns every user ID in the org that has at least one
// active WebSocket connection. Used by ListUsers for the online-only
// filter and the online-count badge.
func (h *Hub) OnlineUserIDs(orgID uuid.UUID) []uuid.UUID {
	h.mu.RLock()
	defer h.mu.RUnlock()

	orgClients, ok := h.clients[orgID]
	if !ok {
		return nil
	}
	ids := make([]uuid.UUID, 0, len(orgClients))
	for uid, clients := range orgClients {
		if len(clients) > 0 {
			ids = append(ids, uid)
		}
	}
	return ids
}

// FilterOnlineUsers returns only the user IDs that have active WebSocket connections.
func (h *Hub) FilterOnlineUsers(orgID uuid.UUID, userIDs []uuid.UUID) []uuid.UUID {
	h.mu.RLock()
	defer h.mu.RUnlock()

	orgClients, ok := h.clients[orgID]
	if !ok {
		return nil
	}

	online := make([]uuid.UUID, 0, len(userIDs))
	for _, uid := range userIDs {
		if userClients, ok := orgClients[uid]; ok && len(userClients) > 0 {
			online = append(online, uid)
		}
	}
	return online
}

// Register adds a client to the hub via the register channel
// Both sends select on quit: once Run has returned nothing drains these
// unbuffered channels, and a bare send would park the caller's goroutine
// forever. After Stop the hub has already closed every client's send channel,
// so dropping the request is correct.
func (h *Hub) Register(client *Client) {
	select {
	case h.register <- client:
	case <-h.quit:
	}
}

// Unregister removes a client from the hub via the unregister channel
func (h *Hub) Unregister(client *Client) {
	select {
	case h.unregister <- client:
	case <-h.quit:
	}
}
