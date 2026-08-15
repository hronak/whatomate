package websocket_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/websocket"
	"github.com/stretchr/testify/assert"
)

// --- IsUserOnline ---

func TestHub_IsUserOnline_ReturnsTrueForRegisteredUser(t *testing.T) {
	hub := newTestHub(t)
	orgID := uuid.New()
	userID := uuid.New()

	hub.Register(newTestClient(hub, userID, orgID))
	waitForClientCount(t, hub, 1)

	assert.True(t, hub.IsUserOnline(orgID, userID))
}

func TestHub_IsUserOnline_FalseForUnknownUser(t *testing.T) {
	hub := newTestHub(t)
	orgID := uuid.New()

	hub.Register(newTestClient(hub, uuid.New(), orgID))
	waitForClientCount(t, hub, 1)

	assert.False(t, hub.IsUserOnline(orgID, uuid.New()))
}

func TestHub_IsUserOnline_FalseForUnknownOrg(t *testing.T) {
	hub := newTestHub(t)
	orgID := uuid.New()
	userID := uuid.New()

	hub.Register(newTestClient(hub, userID, orgID))
	waitForClientCount(t, hub, 1)

	// Same user but different org → false.
	assert.False(t, hub.IsUserOnline(uuid.New(), userID))
}

func TestHub_IsUserOnline_FalseAfterAllClientsUnregister(t *testing.T) {
	hub := newTestHub(t)
	orgID := uuid.New()
	userID := uuid.New()

	c := newTestClient(hub, userID, orgID)
	hub.Register(c)
	waitForClientCount(t, hub, 1)

	hub.Unregister(c)
	waitForClientCount(t, hub, 0)

	assert.False(t, hub.IsUserOnline(orgID, userID))
}

// --- FilterOnlineUsers ---

func TestHub_FilterOnlineUsers_ReturnsOnlyOnlineSubset(t *testing.T) {
	hub := newTestHub(t)
	orgID := uuid.New()
	online1 := uuid.New()
	online2 := uuid.New()
	offline := uuid.New()

	hub.Register(newTestClient(hub, online1, orgID))
	hub.Register(newTestClient(hub, online2, orgID))
	waitForClientCount(t, hub, 2)

	got := hub.FilterOnlineUsers(orgID, []uuid.UUID{online1, offline, online2})
	assert.ElementsMatch(t, []uuid.UUID{online1, online2}, got)
}

func TestHub_FilterOnlineUsers_NilForUnknownOrg(t *testing.T) {
	hub := newTestHub(t)
	got := hub.FilterOnlineUsers(uuid.New(), []uuid.UUID{uuid.New()})
	assert.Nil(t, got)
}

func TestHub_FilterOnlineUsers_EmptyInputReturnsEmpty(t *testing.T) {
	hub := newTestHub(t)
	orgID := uuid.New()
	hub.Register(newTestClient(hub, uuid.New(), orgID))
	waitForClientCount(t, hub, 1)

	got := hub.FilterOnlineUsers(orgID, nil)
	assert.Empty(t, got)
}

// --- BroadcastToUsers ---

// drainCount returns how many messages are pending on the client's send channel.
func drainCount(c *websocket.Client) int {
	got := 0
	ch := c.SendChan()
	for {
		select {
		case <-ch:
			got++
		default:
			return got
		}
	}
}

// awaitMessage blocks until one message reaches the client, failing the test if
// none arrives. Waiting for the delivery we expect is what makes the *negative*
// assertions in these tests meaningful: the hub dispatches a broadcast in a
// single pass, so once the intended recipients have their message, a client
// that has not received one never will. Sleeping instead only ever proved that
// the message had not arrived *yet*.
func awaitMessage(t *testing.T, c *websocket.Client) {
	t.Helper()
	select {
	case <-c.SendChan():
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for a broadcast to reach the client")
	}
}

func TestHub_BroadcastToUsers_DeliversToEachListedUser(t *testing.T) {
	hub := newTestHub(t)
	orgID := uuid.New()
	user1 := uuid.New()
	user2 := uuid.New()
	user3 := uuid.New() // not in the broadcast list

	c1 := newTestClient(hub, user1, orgID)
	c2 := newTestClient(hub, user2, orgID)
	c3 := newTestClient(hub, user3, orgID)
	hub.Register(c1)
	hub.Register(c2)
	hub.Register(c3)
	waitForClientCount(t, hub, 3)

	hub.BroadcastToUsers(orgID, []uuid.UUID{user1, user2}, websocket.WSMessage{Type: "x"})

	// Wait for the two intended recipients rather than sleeping. The dispatch
	// is one pass over the org's clients, so by the time both have their
	// message the third has been passed over for good.
	awaitMessage(t, c1)
	awaitMessage(t, c2)

	assert.Equal(t, 0, drainCount(c1))
	assert.Equal(t, 0, drainCount(c2))
	assert.Equal(t, 0, drainCount(c3), "user not in the list must not receive the message")
}

func TestHub_BroadcastToUsers_NilListIsNoop(t *testing.T) {
	hub := newTestHub(t)
	orgID := uuid.New()
	c := newTestClient(hub, uuid.New(), orgID)
	hub.Register(c)
	waitForClientCount(t, hub, 1)

	hub.BroadcastToUsers(orgID, nil, websocket.WSMessage{Type: "noop"})

	// Fence: send a broadcast that *must* arrive, and wait for it. The hub
	// processes its queue in order, so once the fence has been delivered the
	// nil-list broadcast has definitively been handled — and delivered nothing.
	// A sleep here could only ever say "nothing yet".
	hub.BroadcastToOrg(orgID, websocket.WSMessage{Type: "fence"})
	awaitMessage(t, c)

	assert.Equal(t, 0, drainCount(c), "a nil user list must deliver to nobody")
}
