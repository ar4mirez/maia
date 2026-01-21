package replication

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestElection_NewElection(t *testing.T) {
	cfg := &ElectionConfig{
		NodeID:             "node-1",
		MinElectionTimeout: 100 * time.Millisecond,
		MaxElectionTimeout: 200 * time.Millisecond,
		HeartbeatInterval:  50 * time.Millisecond,
		Priority:           1,
		Nodes:              []string{"node-1"},
	}

	election := NewElection(cfg, zap.NewNop())

	assert.NotNil(t, election)
	assert.Equal(t, ElectionStateFollower, election.State())
	assert.Equal(t, uint64(0), election.Term())
	assert.Equal(t, "", election.LeaderID())
	assert.False(t, election.IsLeader())
}

func TestElection_DefaultConfig(t *testing.T) {
	election := NewElection(nil, nil)

	assert.NotNil(t, election)
	assert.Equal(t, ElectionStateFollower, election.State())
}

func TestElection_RequestVote_GrantVote(t *testing.T) {
	cfg := &ElectionConfig{
		NodeID:             "node-1",
		MinElectionTimeout: 100 * time.Millisecond,
		MaxElectionTimeout: 200 * time.Millisecond,
		Nodes:              []string{"node-1", "node-2"},
	}

	election := NewElection(cfg, zap.NewNop())

	req := &VoteRequest{
		Term:        1,
		CandidateID: "node-2",
		Priority:    1,
	}

	resp, err := election.RequestVote(context.Background(), req)

	require.NoError(t, err)
	assert.True(t, resp.VoteGranted)
	assert.Equal(t, uint64(1), resp.Term)
	assert.Equal(t, "node-1", resp.VoterID)
}

func TestElection_RequestVote_RejectOldTerm(t *testing.T) {
	cfg := &ElectionConfig{
		NodeID:             "node-1",
		MinElectionTimeout: 100 * time.Millisecond,
		MaxElectionTimeout: 200 * time.Millisecond,
		Nodes:              []string{"node-1", "node-2"},
	}

	election := NewElection(cfg, zap.NewNop())

	// First, update the term
	election.ReceiveHeartbeat("node-2", 5)

	// Request with old term
	req := &VoteRequest{
		Term:        3,
		CandidateID: "node-3",
	}

	resp, err := election.RequestVote(context.Background(), req)

	require.NoError(t, err)
	assert.False(t, resp.VoteGranted)
	assert.Equal(t, uint64(5), resp.Term)
}

func TestElection_RequestVote_AlreadyVoted(t *testing.T) {
	cfg := &ElectionConfig{
		NodeID:             "node-1",
		MinElectionTimeout: 100 * time.Millisecond,
		MaxElectionTimeout: 200 * time.Millisecond,
		Nodes:              []string{"node-1", "node-2", "node-3"},
	}

	election := NewElection(cfg, zap.NewNop())

	// Vote for node-2
	req1 := &VoteRequest{
		Term:        1,
		CandidateID: "node-2",
	}
	resp1, _ := election.RequestVote(context.Background(), req1)
	assert.True(t, resp1.VoteGranted)

	// node-3 requests vote for same term
	req2 := &VoteRequest{
		Term:        1,
		CandidateID: "node-3",
	}
	resp2, _ := election.RequestVote(context.Background(), req2)

	// Should not grant - already voted for node-2
	assert.False(t, resp2.VoteGranted)
}

func TestElection_RequestVote_CanVoteForSameCandidateAgain(t *testing.T) {
	cfg := &ElectionConfig{
		NodeID: "node-1",
		Nodes:  []string{"node-1", "node-2"},
	}

	election := NewElection(cfg, zap.NewNop())

	req := &VoteRequest{
		Term:        1,
		CandidateID: "node-2",
	}

	// First vote
	resp1, _ := election.RequestVote(context.Background(), req)
	assert.True(t, resp1.VoteGranted)

	// Vote again for same candidate
	resp2, _ := election.RequestVote(context.Background(), req)
	assert.True(t, resp2.VoteGranted)
}

func TestElection_ReceiveHeartbeat(t *testing.T) {
	cfg := &ElectionConfig{
		NodeID: "node-1",
		Nodes:  []string{"node-1", "node-2"},
	}

	election := NewElection(cfg, zap.NewNop())

	err := election.ReceiveHeartbeat("node-2", 1)

	require.NoError(t, err)
	assert.Equal(t, uint64(1), election.Term())
	assert.Equal(t, "node-2", election.LeaderID())
	assert.Equal(t, ElectionStateFollower, election.State())
}

func TestElection_ReceiveHeartbeat_HigherTerm(t *testing.T) {
	cfg := &ElectionConfig{
		NodeID: "node-1",
		Nodes:  []string{"node-1", "node-2"},
	}

	election := NewElection(cfg, zap.NewNop())
	election.ForceLeadership() // Become leader

	// Receive heartbeat from node with higher term
	err := election.ReceiveHeartbeat("node-2", election.Term()+1)

	require.NoError(t, err)
	assert.Equal(t, "node-2", election.LeaderID())
	assert.Equal(t, ElectionStateFollower, election.State())
	assert.False(t, election.IsLeader())
}

func TestElection_ReceiveHeartbeat_OldTerm(t *testing.T) {
	cfg := &ElectionConfig{
		NodeID: "node-1",
		Nodes:  []string{"node-1", "node-2"},
	}

	election := NewElection(cfg, zap.NewNop())
	election.ReceiveHeartbeat("node-2", 5) // Set term to 5

	// Receive heartbeat with old term
	err := election.ReceiveHeartbeat("node-3", 3)

	assert.ErrorIs(t, err, ErrHigherTerm)
	assert.Equal(t, "node-2", election.LeaderID()) // Still node-2
}

func TestElection_ForceLeadership(t *testing.T) {
	cfg := &ElectionConfig{
		NodeID: "node-1",
		Nodes:  []string{"node-1"},
	}

	election := NewElection(cfg, zap.NewNop())

	leaderCallbackCalled := false
	election.SetCallbacks(
		func() { leaderCallbackCalled = true },
		nil,
		nil,
	)

	election.ForceLeadership()

	assert.True(t, election.IsLeader())
	assert.Equal(t, ElectionStateLeader, election.State())
	assert.Equal(t, "node-1", election.LeaderID())
	assert.True(t, leaderCallbackCalled)
}

func TestElection_StepDown(t *testing.T) {
	cfg := &ElectionConfig{
		NodeID: "node-1",
		Nodes:  []string{"node-1"},
	}

	election := NewElection(cfg, zap.NewNop())

	followerCallbackCalled := false
	election.SetCallbacks(
		nil,
		func(leaderID string) { followerCallbackCalled = true },
		nil,
	)

	election.ForceLeadership()
	assert.True(t, election.IsLeader())

	election.StepDown()

	assert.False(t, election.IsLeader())
	assert.Equal(t, ElectionStateFollower, election.State())
	assert.True(t, followerCallbackCalled)
}

func TestElection_StepDown_NotLeader(t *testing.T) {
	cfg := &ElectionConfig{
		NodeID: "node-1",
		Nodes:  []string{"node-1"},
	}

	election := NewElection(cfg, zap.NewNop())

	// StepDown when not leader should be no-op
	election.StepDown()

	assert.Equal(t, ElectionStateFollower, election.State())
}

func TestElection_AddVote(t *testing.T) {
	cfg := &ElectionConfig{
		NodeID: "node-1",
		Nodes:  []string{"node-1", "node-2", "node-3"},
	}

	election := NewElection(cfg, zap.NewNop())

	// Manually set to candidate state
	election.mu.Lock()
	election.state = ElectionStateCandidate
	election.term = 1
	election.votes = map[string]bool{"node-1": true}
	election.mu.Unlock()

	leaderCallbackCalled := false
	election.SetCallbacks(
		func() { leaderCallbackCalled = true },
		nil,
		nil,
	)

	// Add vote from node-2
	election.AddVote("node-2", 1, true)

	// Should become leader with 2/3 votes
	assert.True(t, election.IsLeader())
	assert.True(t, leaderCallbackCalled)
}

func TestElection_AddVote_WrongTerm(t *testing.T) {
	cfg := &ElectionConfig{
		NodeID: "node-1",
		Nodes:  []string{"node-1", "node-2", "node-3"},
	}

	election := NewElection(cfg, zap.NewNop())

	election.mu.Lock()
	election.state = ElectionStateCandidate
	election.term = 1
	election.votes = map[string]bool{"node-1": true}
	election.mu.Unlock()

	// Add vote from wrong term
	election.AddVote("node-2", 2, true)

	// Should still be candidate
	assert.Equal(t, ElectionStateCandidate, election.State())
}

func TestElection_AddVote_NotCandidate(t *testing.T) {
	cfg := &ElectionConfig{
		NodeID: "node-1",
		Nodes:  []string{"node-1", "node-2"},
	}

	election := NewElection(cfg, zap.NewNop())

	// Not in candidate state
	election.AddVote("node-2", 1, true)

	// Should still be follower
	assert.Equal(t, ElectionStateFollower, election.State())
}

func TestElection_Info(t *testing.T) {
	cfg := &ElectionConfig{
		NodeID: "node-1",
		Nodes:  []string{"node-1", "node-2"},
	}

	election := NewElection(cfg, zap.NewNop())
	election.ReceiveHeartbeat("node-2", 5)

	info := election.Info()

	assert.Equal(t, ElectionStateFollower, info.State)
	assert.Equal(t, uint64(5), info.Term)
	assert.Equal(t, "node-2", info.LeaderID)
	assert.Equal(t, "node-1", info.NodeID)
}

func TestElection_Callbacks(t *testing.T) {
	cfg := &ElectionConfig{
		NodeID: "node-1",
		Nodes:  []string{"node-1"},
	}

	election := NewElection(cfg, zap.NewNop())

	termChanged := false
	var newTerm uint64

	election.SetCallbacks(
		nil,
		nil,
		func(term uint64) {
			termChanged = true
			newTerm = term
		},
	)

	// Trigger term change via heartbeat
	election.ReceiveHeartbeat("node-2", 10)

	assert.True(t, termChanged)
	assert.Equal(t, uint64(10), newTerm)
}

func TestElection_HasQuorum(t *testing.T) {
	testCases := []struct {
		name      string
		nodes     []string
		votes     int
		hasQuorum bool
	}{
		{"single node", []string{"node-1"}, 1, true},
		{"3 nodes, 1 vote", []string{"node-1", "node-2", "node-3"}, 1, false},
		{"3 nodes, 2 votes", []string{"node-1", "node-2", "node-3"}, 2, true},
		{"5 nodes, 2 votes", []string{"n1", "n2", "n3", "n4", "n5"}, 2, false},
		{"5 nodes, 3 votes", []string{"n1", "n2", "n3", "n4", "n5"}, 3, true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &ElectionConfig{
				NodeID: "node-1",
				Nodes:  tc.nodes,
			}
			election := NewElection(cfg, zap.NewNop())

			election.mu.Lock()
			for i := 0; i < tc.votes; i++ {
				election.votes[tc.nodes[i]] = true
			}
			result := election.hasQuorum()
			election.mu.Unlock()

			assert.Equal(t, tc.hasQuorum, result)
		})
	}
}
