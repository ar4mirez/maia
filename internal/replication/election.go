package replication

import (
	"context"
	"errors"
	"math/rand"
	"sync"
	"time"

	"go.uber.org/zap"
)

// Election errors.
var (
	ErrNotCandidate     = errors.New("not in candidate state")
	ErrElectionTimeout  = errors.New("election timed out")
	ErrAlreadyLeader    = errors.New("already a leader")
	ErrAlreadyFollower  = errors.New("already a follower")
	ErrNoQuorum         = errors.New("failed to achieve quorum")
	ErrHigherTerm       = errors.New("received higher term")
	ErrVoteAlreadyCast  = errors.New("vote already cast for this term")
)

// ElectionState represents the state in the leader election process.
type ElectionState string

const (
	// ElectionStateFollower is the initial state - following a leader.
	ElectionStateFollower ElectionState = "follower"

	// ElectionStateCandidate is when requesting votes to become leader.
	ElectionStateCandidate ElectionState = "candidate"

	// ElectionStateLeader is when this node is the leader.
	ElectionStateLeader ElectionState = "leader"
)

// VoteRequest represents a request for a vote in leader election.
type VoteRequest struct {
	// Term is the candidate's term.
	Term uint64 `json:"term"`

	// CandidateID is the candidate requesting the vote.
	CandidateID string `json:"candidate_id"`

	// LastLogIndex is the index of candidate's last log entry.
	LastLogIndex uint64 `json:"last_log_index"`

	// LastLogTerm is the term of candidate's last log entry.
	LastLogTerm uint64 `json:"last_log_term"`

	// Priority is the candidate's priority (higher = more preferred).
	Priority int `json:"priority"`
}

// VoteResponse represents a response to a vote request.
type VoteResponse struct {
	// Term is the current term, for candidate to update itself.
	Term uint64 `json:"term"`

	// VoteGranted is true if the vote was granted.
	VoteGranted bool `json:"vote_granted"`

	// VoterID is the ID of the responding node.
	VoterID string `json:"voter_id"`
}

// ElectionConfig configures the leader election process.
type ElectionConfig struct {
	// NodeID is the unique identifier for this node.
	NodeID string

	// MinElectionTimeout is the minimum election timeout.
	MinElectionTimeout time.Duration

	// MaxElectionTimeout is the maximum election timeout.
	MaxElectionTimeout time.Duration

	// HeartbeatInterval is how often the leader sends heartbeats.
	HeartbeatInterval time.Duration

	// Priority for leader election (higher = more likely to be elected).
	Priority int

	// Nodes is the list of all node IDs in the cluster.
	Nodes []string
}

// DefaultElectionConfig returns default election configuration.
func DefaultElectionConfig() *ElectionConfig {
	return &ElectionConfig{
		MinElectionTimeout: 150 * time.Millisecond,
		MaxElectionTimeout: 300 * time.Millisecond,
		HeartbeatInterval:  50 * time.Millisecond,
		Priority:           1,
	}
}

// Election implements a simplified Raft-like leader election.
type Election struct {
	cfg       *ElectionConfig
	state     ElectionState
	term      uint64
	votedFor  string
	leaderID  string
	votes     map[string]bool
	mu        sync.RWMutex
	logger    *zap.Logger

	// Callbacks
	onBecomeLeader   func()
	onBecomeFollower func(leaderID string)
	onTermChange     func(term uint64)

	// Channels for state machine
	electionTimer  *time.Timer
	heartbeatTimer *time.Timer
	stopCh         chan struct{}
	resetTimerCh   chan struct{}
}

// NewElection creates a new Election instance.
func NewElection(cfg *ElectionConfig, logger *zap.Logger) *Election {
	if cfg == nil {
		cfg = DefaultElectionConfig()
	}

	// Apply defaults for unset values
	if cfg.MinElectionTimeout == 0 {
		cfg.MinElectionTimeout = 150 * time.Millisecond
	}
	if cfg.MaxElectionTimeout == 0 {
		cfg.MaxElectionTimeout = 300 * time.Millisecond
	}
	if cfg.HeartbeatInterval == 0 {
		cfg.HeartbeatInterval = 50 * time.Millisecond
	}

	if logger == nil {
		logger = zap.NewNop()
	}

	return &Election{
		cfg:          cfg,
		state:        ElectionStateFollower,
		term:         0,
		votes:        make(map[string]bool),
		logger:       logger,
		stopCh:       make(chan struct{}),
		resetTimerCh: make(chan struct{}, 1),
	}
}

// SetCallbacks sets the election callbacks.
func (e *Election) SetCallbacks(
	onBecomeLeader func(),
	onBecomeFollower func(leaderID string),
	onTermChange func(term uint64),
) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.onBecomeLeader = onBecomeLeader
	e.onBecomeFollower = onBecomeFollower
	e.onTermChange = onTermChange
}

// Start begins the election process.
func (e *Election) Start(ctx context.Context) error {
	e.mu.Lock()
	e.state = ElectionStateFollower
	e.mu.Unlock()

	// Start election timer
	e.resetElectionTimer()

	go e.runElectionLoop(ctx)

	return nil
}

// Stop stops the election process.
func (e *Election) Stop() {
	close(e.stopCh)
	if e.electionTimer != nil {
		e.electionTimer.Stop()
	}
	if e.heartbeatTimer != nil {
		e.heartbeatTimer.Stop()
	}
}

// State returns the current election state.
func (e *Election) State() ElectionState {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.state
}

// Term returns the current term.
func (e *Election) Term() uint64 {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.term
}

// LeaderID returns the current leader ID.
func (e *Election) LeaderID() string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.leaderID
}

// IsLeader returns true if this node is the leader.
func (e *Election) IsLeader() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.state == ElectionStateLeader
}

// RequestVote handles a vote request from a candidate.
func (e *Election) RequestVote(ctx context.Context, req *VoteRequest) (*VoteResponse, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	resp := &VoteResponse{
		Term:        e.term,
		VoteGranted: false,
		VoterID:     e.cfg.NodeID,
	}

	// If request term is higher, update our term and become follower
	if req.Term > e.term {
		e.term = req.Term
		e.votedFor = ""
		e.state = ElectionStateFollower
		if e.onTermChange != nil {
			e.onTermChange(e.term)
		}
	}

	// Don't grant vote if request term is lower
	if req.Term < e.term {
		return resp, nil
	}

	// Grant vote if we haven't voted or already voted for this candidate
	if e.votedFor == "" || e.votedFor == req.CandidateID {
		// In a full Raft implementation, we'd also check log completeness
		e.votedFor = req.CandidateID
		resp.VoteGranted = true
		resp.Term = e.term

		// Reset election timer when granting vote
		e.resetElectionTimerLocked()

		e.logger.Debug("granted vote",
			zap.String("candidate", req.CandidateID),
			zap.Uint64("term", req.Term))
	}

	return resp, nil
}

// ReceiveHeartbeat handles a heartbeat from the leader.
func (e *Election) ReceiveHeartbeat(leaderID string, term uint64) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	// If term is higher, update and become follower
	if term > e.term {
		e.term = term
		e.votedFor = ""
		e.leaderID = leaderID
		e.state = ElectionStateFollower
		if e.onTermChange != nil {
			e.onTermChange(e.term)
		}
		if e.onBecomeFollower != nil {
			e.onBecomeFollower(leaderID)
		}
	}

	// Ignore heartbeats from old terms
	if term < e.term {
		return ErrHigherTerm
	}

	// Accept heartbeat - reset election timer
	e.leaderID = leaderID
	if e.state != ElectionStateFollower {
		e.state = ElectionStateFollower
		if e.onBecomeFollower != nil {
			e.onBecomeFollower(leaderID)
		}
	}
	e.resetElectionTimerLocked()

	return nil
}

// ForceLeadership forces this node to become leader (for testing/manual failover).
func (e *Election) ForceLeadership() {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.term++
	e.state = ElectionStateLeader
	e.leaderID = e.cfg.NodeID
	e.votedFor = e.cfg.NodeID

	e.logger.Info("forced leadership",
		zap.String("node_id", e.cfg.NodeID),
		zap.Uint64("term", e.term))

	if e.onBecomeLeader != nil {
		e.onBecomeLeader()
	}
}

// StepDown voluntarily steps down from leadership.
func (e *Election) StepDown() {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.state != ElectionStateLeader {
		return
	}

	e.state = ElectionStateFollower
	e.leaderID = ""
	e.votedFor = ""

	e.logger.Info("stepped down from leadership",
		zap.String("node_id", e.cfg.NodeID),
		zap.Uint64("term", e.term))

	if e.onBecomeFollower != nil {
		e.onBecomeFollower("")
	}

	e.resetElectionTimerLocked()
}

// runElectionLoop runs the main election loop.
func (e *Election) runElectionLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-e.stopCh:
			return
		case <-e.electionTimer.C:
			e.handleElectionTimeout()
		case <-e.resetTimerCh:
			// Timer already reset, continue
		}
	}
}

// handleElectionTimeout handles an election timeout.
func (e *Election) handleElectionTimeout() {
	e.mu.Lock()

	// Only start election if we're a follower or candidate
	if e.state == ElectionStateLeader {
		e.mu.Unlock()
		return
	}

	// Become candidate
	e.state = ElectionStateCandidate
	e.term++
	e.votedFor = e.cfg.NodeID
	e.votes = map[string]bool{e.cfg.NodeID: true}
	currentTerm := e.term

	e.logger.Info("starting election",
		zap.String("node_id", e.cfg.NodeID),
		zap.Uint64("term", currentTerm))

	e.mu.Unlock()

	// Request votes from all other nodes
	// In a real implementation, this would be done via HTTP/RPC
	// For now, we'll check if we have a majority with just our own vote

	// Check if we won (for single-node or majority achieved)
	e.mu.Lock()
	if e.state == ElectionStateCandidate && e.term == currentTerm {
		if e.hasQuorum() {
			e.state = ElectionStateLeader
			e.leaderID = e.cfg.NodeID

			e.logger.Info("became leader",
				zap.String("node_id", e.cfg.NodeID),
				zap.Uint64("term", e.term))

			if e.onBecomeLeader != nil {
				e.onBecomeLeader()
			}
		} else {
			// Didn't win, reset timer and try again
			e.resetElectionTimerLocked()
		}
	}
	e.mu.Unlock()
}

// hasQuorum checks if we have enough votes.
func (e *Election) hasQuorum() bool {
	totalNodes := len(e.cfg.Nodes)
	if totalNodes == 0 {
		totalNodes = 1 // Single node cluster
	}
	needed := (totalNodes / 2) + 1
	return len(e.votes) >= needed
}

// AddVote adds a vote from a node.
func (e *Election) AddVote(voterID string, term uint64, granted bool) {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Ignore votes from different terms
	if term != e.term {
		return
	}

	// Only count votes if we're a candidate
	if e.state != ElectionStateCandidate {
		return
	}

	if granted {
		e.votes[voterID] = true
		e.logger.Debug("received vote",
			zap.String("from", voterID),
			zap.Int("total_votes", len(e.votes)))

		// Check if we've won
		if e.hasQuorum() {
			e.state = ElectionStateLeader
			e.leaderID = e.cfg.NodeID

			e.logger.Info("became leader",
				zap.String("node_id", e.cfg.NodeID),
				zap.Uint64("term", e.term))

			if e.onBecomeLeader != nil {
				e.onBecomeLeader()
			}
		}
	}
}

// resetElectionTimer resets the election timer with a random timeout.
func (e *Election) resetElectionTimer() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.resetElectionTimerLocked()
}

// resetElectionTimerLocked resets the timer (must be called with lock held).
func (e *Election) resetElectionTimerLocked() {
	timeout := e.randomElectionTimeout()

	if e.electionTimer == nil {
		e.electionTimer = time.NewTimer(timeout)
	} else {
		if !e.electionTimer.Stop() {
			select {
			case <-e.electionTimer.C:
			default:
			}
		}
		e.electionTimer.Reset(timeout)
	}

	// Signal that timer was reset
	select {
	case e.resetTimerCh <- struct{}{}:
	default:
	}
}

// randomElectionTimeout returns a random timeout between min and max.
func (e *Election) randomElectionTimeout() time.Duration {
	minMs := e.cfg.MinElectionTimeout.Milliseconds()
	maxMs := e.cfg.MaxElectionTimeout.Milliseconds()
	randMs := minMs + rand.Int63n(maxMs-minMs)
	return time.Duration(randMs) * time.Millisecond
}

// ElectionInfo returns information about the current election state.
type ElectionInfo struct {
	State    ElectionState `json:"state"`
	Term     uint64        `json:"term"`
	LeaderID string        `json:"leader_id,omitempty"`
	VotedFor string        `json:"voted_for,omitempty"`
	NodeID   string        `json:"node_id"`
}

// Info returns the current election info.
func (e *Election) Info() *ElectionInfo {
	e.mu.RLock()
	defer e.mu.RUnlock()

	return &ElectionInfo{
		State:    e.state,
		Term:     e.term,
		LeaderID: e.leaderID,
		VotedFor: e.votedFor,
		NodeID:   e.cfg.NodeID,
	}
}
