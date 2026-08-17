package calling

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/pion/webrtc/v4"
	"github.com/redis/go-redis/v9"
	"github.com/shridarpatil/whatomate/internal/assignment"
	"github.com/shridarpatil/whatomate/internal/config"
	"github.com/shridarpatil/whatomate/internal/flowgraph"
	"github.com/shridarpatil/whatomate/internal/models"
	"github.com/shridarpatil/whatomate/internal/storage"
	"github.com/shridarpatil/whatomate/internal/websocket"
	"github.com/shridarpatil/whatomate/pkg/whatsapp"
	"github.com/zerodha/logf"
	"gorm.io/gorm"
)

// signal is a one-shot broadcast used for the call lifecycle events that more
// than one goroutine watches (bridge handover, transfer acceptance, session
// teardown).
//
// Fire is idempotent, so the rotation paths that race to end a transfer can no
// longer double-close and panic. Every method tolerates a nil receiver: Done
// then yields a nil channel, which blocks forever in a select — exactly the
// behaviour of the nil channel fields these replaced.
//
// Reassigning a session's signal (transfer rotation mints a fresh one per
// attempt) hands any existing waiter the previous instance, so a goroutine
// parked on a superseded signal is released by that instance's own Fire rather
// than leaking.
type signal struct {
	ch   chan struct{}
	once sync.Once
}

func newSignal() *signal { return &signal{ch: make(chan struct{})} }

// Fire closes the signal. Safe to call repeatedly and from any goroutine.
func (s *signal) Fire() {
	if s == nil {
		return
	}
	s.once.Do(func() { close(s.ch) })
}

// Done returns a channel closed on the first Fire, or nil if s is nil.
func (s *signal) Done() <-chan struct{} {
	if s == nil {
		return nil
	}
	return s.ch
}

// CallSession represents an active call with its WebRTC state
type CallSession struct {
	ID                string // WhatsApp call_id
	OrganizationID    uuid.UUID
	WhatsAppAccountID *uuid.UUID
	CallerPhone       string
	ContactID         uuid.UUID
	CallLogID         uuid.UUID
	Status            models.CallStatus
	PeerConnection    *webrtc.PeerConnection
	AudioTrack        *webrtc.TrackLocalStaticRTP
	IVRGraph          *IVRFlowGraph
	IVRCtx            *IVRContext
	IVRFlow           *models.IVRFlow
	IVRPlayer         *AudioPlayer // persists across goto_flow for RTP continuity
	DTMFBuffer        chan byte
	StartedAt         time.Time

	// Recording (one per direction for correct OGG/Opus playback)
	CallerRecorder *CallRecorder // caller's audio stream
	AgentRecorder  *CallRecorder // agent's audio stream

	// Transfer HTTP callbacks (configured per-node in IVR flow editor)
	TransferCallbacks *TransferCallbacks

	// Transfer fields
	TransferID        uuid.UUID
	TransferStatus    models.CallTransferStatus
	AgentPC           *webrtc.PeerConnection
	AgentAudioTrack   *webrtc.TrackLocalStaticRTP
	CallerRemoteTrack *webrtc.TrackRemote
	AgentRemoteTrack  *webrtc.TrackRemote
	Bridge            *AudioBridge
	HoldPlayer        *AudioPlayer
	TransferCancel    context.CancelFunc
	BridgeStarted     *signal     // fired when bridge takes over caller track
	TransferAccepted  *signal     // fired when an agent accepts the transfer (rotation signal)
	TransferDone      chan string // outcome sent when transfer ends; nil = terminal
	LastRTPSeq        uint16      // last RTP seq from bridge, for post-transfer player
	LastRTPTimestamp  uint32      // last RTP timestamp from bridge

	// Ringback (outgoing calls)
	RingbackPlayer *AudioPlayer

	// Sticky agent for voice_call-button incoming calls. When set,
	// HandleIncomingCall skips the IVR load and the post-media branch in
	// webrtc.go kicks off a transfer to this agent directly (see
	// initiateTransfer, which prefers this over the contact's
	// assigned_user_id).
	StickyAgentID *uuid.UUID

	// Outgoing call fields
	Direction      models.CallDirection
	AgentID        uuid.UUID
	TargetPhone    string
	WAPeerConn     *webrtc.PeerConnection      // WhatsApp-side PC (outgoing only)
	WAAudioTrack   *webrtc.TrackLocalStaticRTP // server→WhatsApp audio track
	WARemoteTrack  *webrtc.TrackRemote         // WhatsApp's remote audio track
	SDPAnswerReady chan string                 // webhook delivers SDP answer here

	// done is fired once by cleanupSession. It is how consumer goroutines
	// (DTMF waiters, RTP readers) learn the session is gone. It replaces
	// closing DTMFBuffer, which raced with dtmf.go's send.
	done *signal

	mu sync.Mutex
}

// --- Guarded accessors ---
//
// The fields below are written by the transfer/teardown paths while consumer
// goroutines read them, so every access goes through session.mu. Callers take
// a snapshot and then select on it outside the lock; a stale snapshot is safe
// because each signal is fired exactly once by whoever supersedes it.

// bridgeStarted returns the current BridgeStarted signal, which may be nil.
func (s *CallSession) bridgeStarted() *signal {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.BridgeStarted
}

// fireBridgeStarted releases anything waiting on the bridge handover.
func (s *CallSession) fireBridgeStarted() {
	s.mu.Lock()
	sig := s.BridgeStarted
	s.mu.Unlock()
	sig.Fire()
}

// newTransferAccepted installs a fresh TransferAccepted signal and returns it.
func (s *CallSession) newTransferAccepted() *signal {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.TransferAccepted = newSignal()
	return s.TransferAccepted
}

// transferAccepted returns the current TransferAccepted signal, which may be nil.
func (s *CallSession) transferAccepted() *signal {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.TransferAccepted
}

// dtmfChan returns the session's DTMF buffer, or nil once the session has been
// torn down. The channel is never closed, so every receive must also select on
// doneChan to avoid blocking past the end of the call.
func (s *CallSession) dtmfChan() chan byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.DTMFBuffer
}

// callID returns the WhatsApp call ID, which is empty until the outgoing-call
// flow has actually placed the call. Read under the lock because the pion
// callbacks registered before that point consult it.
func (s *CallSession) callID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ID
}

// doneChan returns a channel closed when the session is cleaned up.
func (s *CallSession) doneChan() <-chan struct{} {
	s.mu.Lock()
	sig := s.done
	s.mu.Unlock()
	return sig.Done()
}

// IVRNodeType identifies the kind of applet in an IVR flow graph.
type IVRNodeType string

const (
	IVRNodeGreeting     IVRNodeType = "greeting"
	IVRNodeMenu         IVRNodeType = "menu"
	IVRNodeGather       IVRNodeType = "gather"
	IVRNodeHTTPCallback IVRNodeType = "http_callback"
	IVRNodeTransfer     IVRNodeType = "transfer"
	IVRNodeGotoFlow     IVRNodeType = "goto_flow"
	IVRNodeTiming       IVRNodeType = "timing"
	IVRNodeHangup       IVRNodeType = "hangup"
)

// TransferHTTPCallback holds the configuration for a single transfer lifecycle HTTP callback.
type TransferHTTPCallback struct {
	URL          string            `json:"url"`
	Method       string            `json:"method"`
	Headers      map[string]string `json:"headers"`
	BodyTemplate string            `json:"body_template"`
}

// TransferCallbacks holds optional HTTP callbacks for each transfer lifecycle event.
type TransferCallbacks struct {
	OnWaiting *TransferHTTPCallback
	OnConnect *TransferHTTPCallback
}

// IVRNode, IVREdge and IVRFlowGraph are the IVR domain's views of the shared
// flow-graph types, specialized to IVRNodeType. Edge conditions for the IVR
// engine include "default", "digit:N", "timeout", "max_retries", "http:2xx",
// "http:non2xx", "in_hours", "out_of_hours". Traversal (BuildMaps/Node/
// ResolveEdge/OutgoingEdges) lives in internal/flowgraph.
type (
	IVRNode      = flowgraph.Node[IVRNodeType]
	IVREdge      = flowgraph.Edge
	IVRFlowGraph = flowgraph.Graph[IVRNodeType]
)

// IVRContext holds runtime state during IVR flow execution.
type IVRContext struct {
	Variables   map[string]string
	CallerPhone string
	CallID      string
	CurrentNode string
	Path        []map[string]string
}

// Manager manages active call sessions
type Manager struct {
	sessions map[string]*CallSession
	mu       sync.RWMutex
	log      logf.Logger
	whatsapp *whatsapp.Client
	db       *gorm.DB
	wsHub    *websocket.Hub
	config   *config.CallingConfig
	s3       *storage.S3Client // nil when recording is disabled
	redis    *redis.Client
	assigner *assignment.Assigner
}

// NewManager creates a new call session manager
func NewManager(cfg *config.CallingConfig, s3Client *storage.S3Client, db *gorm.DB, rd *redis.Client, waClient *whatsapp.Client, wsHub *websocket.Hub, assigner *assignment.Assigner, log logf.Logger) *Manager {
	// Apply defaults for server-level config
	if cfg.AudioDir == "" {
		cfg.AudioDir = "./audio"
	}
	if cfg.HoldMusicFile == "" {
		cfg.HoldMusicFile = "hold.ogg"
	}
	if cfg.MaxCallDuration <= 0 {
		cfg.MaxCallDuration = 3600
	}
	if cfg.TransferTimeoutSecs <= 0 {
		cfg.TransferTimeoutSecs = 60
	}

	if cfg.PerAgentTimeoutSecs <= 0 {
		cfg.PerAgentTimeoutSecs = 15
	}

	return &Manager{
		sessions: make(map[string]*CallSession),
		log:      log,
		whatsapp: waClient,
		db:       db,
		redis:    rd,
		wsHub:    wsHub,
		config:   cfg,
		s3:       s3Client,
		assigner: assigner,
	}
}

// HandleIncomingCall processes a new incoming call and starts WebRTC negotiation.
// The sdpOffer parameter is the consumer's SDP offer received from the webhook's
// session.sdp field in the "connect" event.
//
// stickyAgentID, if non-nil, comes from the voice_call button payload — the
// caller pre-validated the agent's eligibility. We bypass the IVR for these
// calls; after WebRTC media connects, webrtc.go kicks off a transfer to this
// agent directly instead of running runIVRFlow.
func (m *Manager) HandleIncomingCall(account *models.WhatsAppAccount, contact *models.Contact, callLog *models.CallLog, sdpOffer string, stickyAgentID *uuid.UUID) {
	session := &CallSession{
		ID:                callLog.WhatsAppCallID,
		OrganizationID:    account.OrganizationID,
		WhatsAppAccountID: &account.ID,
		CallerPhone:       contact.PhoneNumber,
		ContactID:         contact.ID,
		CallLogID:         callLog.ID,
		Status:            models.CallStatusRinging,
		DTMFBuffer:        make(chan byte, 32),
		StartedAt:         time.Now(),
		BridgeStarted:     newSignal(),
		StickyAgentID:     stickyAgentID,
		done:              newSignal(),
	}

	// Load IVR flow if assigned (cached). Skipped for sticky-routed calls —
	// those bypass IVR entirely and go straight to a transfer.
	if stickyAgentID == nil && callLog.IVRFlowID != nil {
		session.IVRFlow = m.getIVRFlowCached(*callLog.IVRFlowID)
	}

	m.mu.Lock()
	m.sessions[session.ID] = session
	m.mu.Unlock()

	m.log.Info("Call session created",
		"call_id", session.ID,
		"caller", session.CallerPhone,
		"has_sdp_offer", sdpOffer != "",
	)

	// Start WebRTC negotiation using the consumer's SDP offer
	go m.negotiateWebRTC(session, account, sdpOffer)
}

// HandleCallEvent processes a call lifecycle event (in_call, ended, etc.)
func (m *Manager) HandleCallEvent(callID, event string) {
	m.mu.RLock()
	session, exists := m.sessions[callID]
	m.mu.RUnlock()

	if !exists {
		return
	}

	session.mu.Lock()
	var action string
	var transferID uuid.UUID

	switch event {
	case "in_call", "connect":
		session.Status = models.CallStatusAnswered
	case "ended", "terminate", "missed", "unanswered":
		switch session.TransferStatus {
		case models.CallTransferStatusWaiting:
			action = "hangup_transfer"
		case models.CallTransferStatusConnected:
			action = "end_transfer"
			transferID = session.TransferID
		default:
			session.Status = models.CallStatusCompleted
			action = "cleanup"
		}
	}
	session.mu.Unlock()

	switch action {
	case "hangup_transfer":
		m.HandleCallerHangupDuringTransfer(session)
	case "end_transfer":
		m.EndTransfer(transferID)
	case "cleanup":
		go m.cleanupSession(callID)
	}
}

// EndCall terminates a call session and cleans up resources
func (m *Manager) EndCall(callID string) {
	m.cleanupSession(callID)
}

// Shutdown ends every active call and returns once their sessions are cleaned
// up, or when ctx expires.
//
// Without this, process shutdown simply dropped live calls: peer connections
// were never closed, in-progress recordings were never finalized or uploaded,
// and the corresponding CallLog rows stayed stuck in their last status.
func (m *Manager) Shutdown(ctx context.Context) {
	m.mu.RLock()
	callIDs := make([]string, 0, len(m.sessions))
	for id := range m.sessions {
		callIDs = append(callIDs, id)
	}
	m.mu.RUnlock()

	if len(callIDs) == 0 {
		return
	}

	m.log.Info("Ending active calls for shutdown", "count", len(callIDs))

	done := make(chan struct{})
	go func() {
		defer close(done)
		for _, id := range callIDs {
			// The session may already be gone if it ended between the snapshot
			// above and now.
			if s := m.GetSession(id); s != nil {
				m.terminateCallBySession(s)
			}
			m.cleanupSession(id)
		}
	}()

	select {
	case <-done:
		m.log.Info("All active calls ended")
	case <-ctx.Done():
		m.log.Warn("Timed out ending active calls", "remaining", len(m.sessions))
	}
}

// GetSession returns a call session by ID
func (m *Manager) GetSession(callID string) *CallSession {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.sessions[callID]
}

// GetSessionByCallLogID returns a call session by its CallLog ID
func (m *Manager) GetSessionByCallLogID(callLogID uuid.UUID) *CallSession {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, s := range m.sessions {
		if s.CallLogID == callLogID {
			return s
		}
	}
	return nil
}

// orgCallingSettings holds per-org calling overrides resolved from a single DB query.
type orgCallingSettings struct {
	TransferTimeoutSecs int
	MaskPhoneNumbers    bool
	HoldMusicFile       string
	RingbackFile        string
}

// transferRingFile returns the audio file the caller should hear while a
// transfer is pending an agent's acceptance. Prefers RingbackFile (sounds
// like "we're connecting you", closer to what a customer expects) and
// falls back to HoldMusicFile when no ringback asset is configured for
// the org. Empty string ⇒ silence.
func (s orgCallingSettings) transferRingFile() string {
	if s.RingbackFile != "" {
		return s.RingbackFile
	}
	return s.HoldMusicFile
}

// getOrgCallingSettings returns cached org-level calling overrides,
// falling back to global config defaults for any missing values.
func (m *Manager) getOrgCallingSettings(orgID uuid.UUID) orgCallingSettings {
	return m.getOrgCallingSettingsCached(orgID)
}

// getOrgRingback returns the ringback file path for a session's organization.
func (m *Manager) getOrgRingback(orgID uuid.UUID) string {
	return m.getOrgCallingSettings(orgID).RingbackFile
}

// cleanupSession removes a session and releases WebRTC resources
func (m *Manager) cleanupSession(callID string) {
	m.mu.Lock()
	session, exists := m.sessions[callID]
	if !exists {
		m.mu.Unlock()
		return
	}

	// If a transfer is in the "waiting" state the agent's PC is being torn
	// down intentionally. Don't destroy the whole session — the caller-side
	// (or WA-side) PeerConnection must stay alive for hold music.
	session.mu.Lock()
	if session.TransferStatus == models.CallTransferStatusWaiting {
		session.mu.Unlock()
		m.mu.Unlock()
		m.log.Info("Skipping cleanup — transfer in waiting state", "call_id", callID)
		return
	}
	session.mu.Unlock()

	delete(m.sessions, callID)
	m.mu.Unlock()

	// Snapshot state and resources under lock, then release before calling external methods
	session.mu.Lock()

	// Transfer state snapshot for DB updates
	transferID := session.TransferID
	transferStatus := session.TransferStatus
	callLogID := session.CallLogID
	orgID := session.OrganizationID

	if transferID != uuid.Nil && transferStatus == models.CallTransferStatusWaiting {
		session.TransferStatus = models.CallTransferStatusAbandoned
	}

	// Snapshot and nil resources to prevent double-close
	bridge := session.Bridge
	session.Bridge = nil
	holdPlayer := session.HoldPlayer
	session.HoldPlayer = nil
	ringbackPlayer := session.RingbackPlayer
	session.RingbackPlayer = nil
	ivrPlayer := session.IVRPlayer
	session.IVRPlayer = nil
	transferCancel := session.TransferCancel
	session.TransferCancel = nil
	agentPC := session.AgentPC
	session.AgentPC = nil
	waPeerConn := session.WAPeerConn
	session.WAPeerConn = nil
	peerConn := session.PeerConnection
	session.PeerConnection = nil
	session.DTMFBuffer = nil
	callerRec := session.CallerRecorder
	session.CallerRecorder = nil
	agentRec := session.AgentRecorder
	session.AgentRecorder = nil
	transferDone := session.TransferDone
	session.TransferDone = nil
	// Release anything still parked on a transfer handshake before the
	// session's resources go away.
	bridgeStarted := session.BridgeStarted
	transferAccepted := session.TransferAccepted
	doneSig := session.done

	session.mu.Unlock()

	// Fire done first: consumer goroutines observe teardown before the
	// resources they hold are torn down under them.
	doneSig.Fire()
	bridgeStarted.Fire()
	transferAccepted.Fire()

	// Close TransferDone to unblock any waiting IVR goroutine
	if transferDone != nil {
		close(transferDone)
	}

	// DB operations and broadcasts (outside lock)
	if transferID != uuid.Nil && transferStatus == models.CallTransferStatusWaiting {
		now := time.Now()
		m.db.Model(&models.CallTransfer{}).
			Where("id = ? AND status = ?", transferID, models.CallTransferStatusWaiting).
			Updates(map[string]any{
				"status":       models.CallTransferStatusAbandoned,
				"completed_at": now,
			})
		m.db.Model(&models.CallLog{}).
			Where("id = ?", callLogID).
			Update("disconnected_by", models.DisconnectedByClient)
		m.broadcastEvent(orgID, websocket.TypeCallTransferAbandoned, map[string]any{
			"id":           transferID.String(),
			"completed_at": now.Format(time.RFC3339),
		})
		m.log.Info("Transfer marked abandoned during cleanup", "transfer_id", transferID, "call_id", callID)
	}

	// Stop resources (outside lock)
	if bridge != nil {
		bridge.Stop()
	}
	if holdPlayer != nil {
		holdPlayer.Stop()
	}
	if ringbackPlayer != nil {
		ringbackPlayer.Stop()
	}
	if ivrPlayer != nil {
		ivrPlayer.Stop()
	}
	if transferCancel != nil {
		transferCancel()
	}
	if agentPC != nil {
		if err := agentPC.Close(); err != nil {
			m.log.Error("Failed to close agent peer connection", "error", err, "call_id", callID)
		}
	}

	// Close WhatsApp peer connection (outgoing calls)
	if waPeerConn != nil {
		if err := waPeerConn.Close(); err != nil {
			m.log.Error("Failed to close WA peer connection", "error", err, "call_id", callID)
		}
	}

	// Close caller peer connection
	if peerConn != nil {
		if err := peerConn.Close(); err != nil {
			m.log.Error("Failed to close peer connection", "error", err, "call_id", callID)
		}
	}

	// DTMFBuffer is deliberately not closed: dtmf.go sends into it from the
	// WebRTC read loop, and closing under it panicked the process. Nilling it
	// under the lock (above) plus firing done releases every consumer.

	// Finalize recording (async — don't block cleanup)
	if callerRec != nil || agentRec != nil {
		go m.finalizeRecording(orgID, callLogID, callerRec, agentRec)
	}

	m.log.Info("Call session cleaned up", "call_id", callID)
}

// --- Shared helpers to reduce duplication across calling files ---

// broadcastEvent broadcasts a call event via WebSocket to an organization.
func (m *Manager) broadcastEvent(orgID uuid.UUID, eventType string, payload map[string]any) {
	if m.wsHub == nil {
		return
	}
	m.wsHub.BroadcastToOrg(orgID, websocket.WSMessage{
		Type:    eventType,
		Payload: payload,
	})
}

// setupAudioBridge creates per-direction recorders (if enabled), builds an
// AudioBridge, and assigns everything to the session under its lock.
// If recorders already exist on the session (e.g. after a transfer), they are
// reused so the entire call is captured in continuous files.
func (m *Manager) setupAudioBridge(session *CallSession) *AudioBridge {
	session.mu.Lock()
	callerRec := session.CallerRecorder
	agentRec := session.AgentRecorder
	session.mu.Unlock()

	if callerRec == nil {
		callerRec = m.newRecorderIfEnabled()
	}
	if agentRec == nil {
		agentRec = m.newRecorderIfEnabled()
	}

	bridge := NewAudioBridge(callerRec, agentRec)
	session.mu.Lock()
	session.Bridge = bridge
	session.CallerRecorder = callerRec
	session.AgentRecorder = agentRec
	session.mu.Unlock()
	return bridge
}

// terminateCall terminates an active call via the WhatsApp API.
func (m *Manager) terminateCall(session *CallSession, waAccount *whatsapp.Account) {
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := m.whatsapp.TerminateCall(c, waAccount, session.ID); err != nil {
		m.log.Error("Failed to terminate call via API", "error", err, "call_id", session.ID)
	}
}

// terminateCallBySession looks up the WhatsApp account from the DB and
// terminates the call. Used when only the session is available.
func (m *Manager) terminateCallBySession(session *CallSession) {
	var account models.WhatsAppAccount
	if err := m.db.Where("organization_id = ? AND name = ?", session.OrganizationID, session.WhatsAppAccountID).
		First(&account).Error; err != nil {
		m.log.Error("Failed to look up account for call termination", "error", err, "call_id", session.ID)
		return
	}
	waAccount := account.ToWAAccount()
	if waAccount.AccessToken != "" {
		m.terminateCall(session, waAccount)
	}
}

// durationSince calculates seconds elapsed since a given time, returning 0 if
// the pointer is nil.
func durationSince(from *time.Time, now time.Time) int {
	if from == nil {
		return 0
	}
	return int(now.Sub(*from).Seconds())
}

// newRecorderIfEnabled creates a CallRecorder if recording is enabled, or returns nil.
func (m *Manager) newRecorderIfEnabled() *CallRecorder {
	if !m.config.RecordingEnabled || m.s3 == nil {
		return nil
	}
	rec, err := NewCallRecorder()
	if err != nil {
		m.log.Error("Failed to create call recorder", "error", err)
		return nil
	}
	return rec
}

// finalizeRecording stops both per-direction recorders, merges them into a
// single OGG/Opus file using FFmpeg, uploads to S3, and updates the CallLog.
func (m *Manager) finalizeRecording(orgID, callLogID uuid.UUID, callerRec, agentRec *CallRecorder) {
	var callerPath, agentPath string
	var callerCount, agentCount int

	if callerRec != nil {
		var err error
		callerPath, callerCount, err = callerRec.Stop()
		defer func() { _ = os.Remove(callerPath) }()
		if err != nil {
			m.log.Error("Caller recording had write errors", "error", err, "call_log_id", callLogID)
		}
	}
	if agentRec != nil {
		var err error
		agentPath, agentCount, err = agentRec.Stop()
		defer func() { _ = os.Remove(agentPath) }()
		if err != nil {
			m.log.Error("Agent recording had write errors", "error", err, "call_log_id", callLogID)
		}
	}

	m.log.Info("Recording finalized",
		"call_log_id", callLogID,
		"caller_packets", callerCount,
		"agent_packets", agentCount,
		"caller_path", callerPath,
		"agent_path", agentPath,
	)

	maxCount := max(agentCount, callerCount)
	if maxCount == 0 {
		return
	}

	// Duration from the longer stream (each packet = 20ms)
	durationSecs := maxCount * 20 / 1000
	if durationSecs == 0 && maxCount > 0 {
		durationSecs = 1
	}

	// Merge the two direction files into one using FFmpeg.
	// If only one direction was recorded, use it directly.
	var uploadPath string
	switch {
	case callerCount > 0 && agentCount > 0:
		merged, err := mergeRecordings(callerPath, agentPath)
		if err != nil {
			m.log.Error("Failed to merge recordings, uploading caller only",
				"error", err, "call_log_id", callLogID)
			uploadPath = callerPath
		} else {
			defer func() { _ = os.Remove(merged) }()
			uploadPath = merged
		}
	case callerCount > 0:
		uploadPath = callerPath
	default:
		uploadPath = agentPath
	}

	s3Key := fmt.Sprintf("recordings/%s/%s.ogg", orgID.String(), callLogID.String())

	f, err := os.Open(uploadPath)
	if err != nil {
		m.log.Error("Failed to open recording file", "error", err, "call_log_id", callLogID)
		return
	}
	defer f.Close() //nolint:errcheck

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if err := m.s3.Upload(ctx, s3Key, f, "audio/ogg"); err != nil {
		m.log.Error("Failed to upload recording to S3", "error", err, "call_log_id", callLogID)
		if dbErr := m.db.Model(&models.CallLog{}).
			Where("id = ?", callLogID).
			Update("recording_error", err.Error()).Error; dbErr != nil {
			m.log.Error("Failed to update call log with recording error", "error", dbErr, "call_log_id", callLogID)
		}
		return
	}

	if err := m.db.Model(&models.CallLog{}).
		Where("id = ?", callLogID).
		Updates(map[string]any{
			"recording_s3_key":   s3Key,
			"recording_duration": durationSecs,
		}).Error; err != nil {
		m.log.Error("Failed to update call log with recording metadata", "error", err, "s3_key", s3Key, "call_log_id", callLogID)
	}

	m.log.Info("Recording uploaded",
		"call_log_id", callLogID,
		"s3_key", s3Key,
		"caller_packets", callerCount,
		"agent_packets", agentCount,
		"duration_secs", durationSecs,
	)
}

// mergeRecordings uses FFmpeg to mix two mono OGG/Opus files into one.
func mergeRecordings(file1, file2 string) (string, error) {
	out, err := os.CreateTemp("", "call-merged-*.ogg")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	outPath := out.Name()
	_ = out.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	// file1 = caller, file2 = agent. Agent browser mic is typically quieter
	// than WhatsApp's caller audio. Boost agent volume and use loudnorm to
	// level both streams before mixing.
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-i", file1,
		"-i", file2,
		"-filter_complex",
		"[0:a]loudnorm=I=-16:TP=-1.5:LRA=11[a1];[1:a]volume=3,loudnorm=I=-16:TP=-1.5:LRA=11[a2];[a1][a2]amix=inputs=2:duration=longest:normalize=0",
		"-c:a", "libopus",
		"-y", outPath,
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		_ = os.Remove(outPath)
		return "", fmt.Errorf("ffmpeg: %w: %s", err, output)
	}

	return outPath, nil
}

// logWrite reports a failed database write at a site that has no error path to
// return to. Call-log and transfer-state transitions are best-effort from the
// media path's point of view, but a silent failure leaves the row showing a
// status the call is no longer in.
func (m *Manager) logWrite(op string, tx *gorm.DB, kv ...any) {
	if tx.Error != nil {
		m.log.Error("Database write failed", append([]any{"op", op, "error", tx.Error}, kv...)...)
	}
}
