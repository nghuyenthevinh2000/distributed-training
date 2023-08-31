package main

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
	"sync"
	"time"
)

func max(a, b int) int {
	if a > b {
		return a
	}

	return b
}

var raft_lock sync.RWMutex

type State int

const (
	follower                 State = 0
	candidate                State = 1
	leader                   State = 2
	election_timeout               = 2 * time.Second
	heartbeat_interval             = 1 * time.Second
	min_replication_interval       = 500 * time.Millisecond
)

type Raft struct {
	node          *Node
	state_machine *Map
	state         State
	leader        string

	election_deadline  time.Time
	step_down_deadline time.Time
	last_replication   time.Time

	term      float64
	log       *Log
	voted_for string

	// leader state
	commit_index float64
	last_applied float64
	next_index   map[string]int
	match_index  map[string]int
}

func (r *Raft) init() {
	r.node = newNode()
	r.state_machine = &Map{}
	r.state_machine.init()
	r.state = follower

	r.election_deadline = time.Now()
	r.step_down_deadline = time.Now()
	r.last_replication = time.Now()

	r.term = 0
	r.log = &Log{}
	r.log.init(r.node)
	r.voted_for = ""

	r.commit_index = 0
	r.last_applied = 1
	r.next_index = make(map[string]int)
	r.match_index = make(map[string]int)

	r.node.on("read", r.clientRequest)
	r.node.on("write", r.clientRequest)
	r.node.on("cas", r.clientRequest)
	r.node.on("request_vote", r.handleVote)
	r.node.on("append_entries", r.appendEntries)

	// set our node to be candidate if no authority from leader is detected for more than 1 seconds
	getInterval := func() time.Duration {
		return time.Duration(rand.Intn(500) * int(time.Millisecond))
	}

	getMinReplicationInterval := func() time.Duration {
		return min_replication_interval
	}

	r.node.every(getInterval, r.checkAuthority)
	r.node.every(getInterval, r.checkStepDown)
	r.node.every(getMinReplicationInterval, r.replicateLog)
}

func (r *Raft) clientRequest(req Request) error {
	logSafe("handling client request")

	op := req.Body.(map[string]interface{})
	op["src"] = req.Src

	raft_lock.RLock()
	defer raft_lock.RUnlock()
	if r.state != leader {
		if r.leader == "" {
			err := newRPCError(38)
			err.LogError("")

			return err.Error("")
		}

		logSafe(fmt.Sprintf("not leader, forwarding request: %v to leader %s", op, r.leader))

		// if not leader, forward request to leader
		r.node.rpc(r.leader, op, func(leaderReq Request) error {
			// extract result from leader, then send to original requester
			body := leaderReq.Body.(map[string]interface{})

			logSafe(fmt.Sprintf("got result from leader: %v", body))

			r.node.reply(req, body)
			return nil
		})

		return nil
	}

	r.log.appendEntry([]Entry{
		{term: r.term, op: op},
	})

	return nil
}

func (r *Raft) becomeCandidate() {
	r.state = candidate
	r.advanceTerm(r.term + 1)
	r.voted_for = r.node.nodeId
	r.leader = ""
	r.resetElectionDeadline()
	r.resetStepDownDeadline()
	logSafe("became candidate")
	r.requestVotes()
}

func (r *Raft) becomeFollower() {
	r.state = follower
	r.match_index = make(map[string]int)
	r.next_index = make(map[string]int)
	r.leader = ""
	r.resetElectionDeadline()
	logSafe(fmt.Sprintf("became follower for term %f", r.term))
}

func (r *Raft) becomeLeader() {
	if r.state != candidate {
		newRPCError(37).LogError("")
		return
	}

	r.state = leader
	r.leader = ""

	for _, id := range r.node.nodeIds {
		if id == r.node.nodeId {
			continue
		}

		r.next_index[id] = r.log.size() + 1
		r.match_index[id] = 0
	}

	r.resetStepDownDeadline()
	logSafe(fmt.Sprintf("became leader for term %f with log = %v", r.term, r.log.entries))
}

// election deadline will range from [2;3] seconds
func (r *Raft) resetElectionDeadline() {
	rand_time := time.Duration(rand.Intn(1000) * int(time.Millisecond))
	r.election_deadline = time.Now().Add(election_timeout + rand_time)
}

func (r *Raft) resetStepDownDeadline() {
	r.step_down_deadline = time.Now().Add(election_timeout)
}

func (r *Raft) checkAuthority() {
	logSafe("Checking authority")

	raft_lock.Lock()
	defer raft_lock.Unlock()

	if r.election_deadline.Before(time.Now()) {
		if r.state != leader {
			r.becomeCandidate()
		} else {
			r.resetElectionDeadline()
		}
	}
}

func (r *Raft) checkStepDown() {
	logSafe("checking step down")

	raft_lock.Lock()
	defer raft_lock.Unlock()

	if r.state == leader && r.step_down_deadline.Before(time.Now()) {
		logSafe("Stepping down as leader")
		r.becomeFollower()
	}
}

// new term must always be greater than current term
func (r *Raft) advanceTerm(term float64) {
	if r.term > term {
		newRPCError(36).LogError("")
		return
	}

	r.term = term

	// reset voted for because leader is chosen
	r.voted_for = ""
}

func (r *Raft) maybeStepDown(remote_term float64) {
	if r.term < remote_term {
		logSafe(fmt.Sprintf("remote term %f is greater than local term %f", remote_term, r.term))
		r.advanceTerm(remote_term)
		r.becomeFollower()
	}
}

func (r *Raft) requestVotes() {
	logSafe("Requesting votes")

	current_term := r.term
	voteLock := sync.Mutex{}
	votes := map[string]bool{}

	// construct request vote rpc body
	resBody := map[string]interface{}{}
	resBody["type"] = "request_vote"
	resBody["term"] = r.term
	resBody["candidate_id"] = r.node.nodeId
	resBody["last_log_index"] = r.log.size()
	resBody["last_log_term"] = r.log.lastEntry().term

	// send request vote rpc to all other nodes in the cluster
	r.node.brpc(resBody, func(req Request) error {
		logSafe(fmt.Sprintf("got vote from %s", req.Src))

		r.resetStepDownDeadline()
		reqBody := req.Body.(map[string]interface{})
		term := reqBody["term"].(float64)
		r.maybeStepDown(term)
		if r.state == candidate && r.term == current_term && r.term == term && reqBody["vote_granted"].(bool) {
			voteLock.Lock()
			votes[req.Src] = true
			voteLock.Unlock()

			if r.majority(len(votes)) {
				r.becomeLeader()
			}
		}

		return nil
	})
}

func (r *Raft) handleVote(req Request) error {
	logSafe("handling vote")

	body := req.Body.(map[string]interface{})
	reqTerm := body["term"].(float64)
	lastLogTerm := body["last_log_term"].(float64)
	lastLogIndex := int(body["last_log_index"].(float64))

	raft_lock.Lock()
	defer raft_lock.Unlock()

	r.maybeStepDown(reqTerm)

	grant := false
	if reqTerm < r.term {
		logSafe(fmt.Sprintf("request term: %f is smaller than current local term: %f, not granting vote", reqTerm, r.term))
	} else if r.voted_for != "" {
		logSafe(fmt.Sprintf("already voted for %s, not granting vote", r.voted_for))
	} else if lastLogTerm < r.log.lastEntry().term {
		logSafe(fmt.Sprintf("request last log term: %f is smaller than local last log term: %f, not granting vote", lastLogTerm, r.log.lastEntry().term))
	} else if lastLogTerm == r.log.lastEntry().term && lastLogIndex < r.log.size() {
		logSafe(fmt.Sprintf("logs are both at same term %f, but request last log index: %d is smaller than local last log index: %d, not granting vote", lastLogTerm, lastLogIndex, r.log.size()))
	} else {
		candidateId := body["candidate_id"].(string)
		logSafe(fmt.Sprintf("granting vote to %s", candidateId))
		grant = true
		r.voted_for = candidateId
		r.resetElectionDeadline()
	}

	r.node.reply(req, map[string]interface{}{
		"type":         "request_vote_res",
		"term":         r.term,
		"vote_granted": grant,
	})

	return nil
}

// if votes are majority, become leader
func (r *Raft) majority(votes int) bool {
	quorum := (votes / 2) + 1
	return quorum > len(r.node.nodeIds)/2
}

// determint votes median for committing
func (r *Raft) median(data []int) int {
	dataCopy := make([]int, len(data))
	copy(dataCopy, data)

	sort.Ints(dataCopy)

	var median int
	l := len(dataCopy)
	if l == 0 {
		return 0
	} else if l%2 == 0 {
		median = (dataCopy[l/2-1] + dataCopy[l/2]) / 2
	} else {
		median = dataCopy[l/2]
	}

	return median
}

func (r *Raft) matchIndex() {
	r.match_index[r.node.nodeId] = r.log.size()
}

func (r *Raft) advanceCommitIndex() {
	if r.state == leader {
		keys := make([]int, len(r.match_index))
		for _, vals := range r.match_index {
			keys = append(keys, vals)
		}

		n := r.median(keys)

		if r.commit_index < float64(n) && r.log.getEntry(n).term == r.term {
			logSafe(fmt.Sprintf(" commit index now= %v \n", n))
			r.commit_index = float64(n)
		}
	}
	r.advanceStateMachine()
}

func (r *Raft) advanceStateMachine() {
	for r.last_applied < r.commit_index {
		r.last_applied++
		op := r.log.getEntry(int(r.last_applied)).op
		res, err := r.state_machine.apply(op)
		if err != nil {
			logSafe(fmt.Sprintf("Error applying state machine: %s", err))
			continue
		}

		logSafe(fmt.Sprintf("state machine result is = %v", res))

		if r.state == leader {
			req := Request{
				Src:  op["src"].(string),
				Body: op,
			}
			r.node.reply(req, res)
		}
	}
}

func (r *Raft) replicateLog() {
	logSafe("replicating log")

	raft_lock.Lock()
	defer raft_lock.Unlock()

	elapsedTime := time.Since(r.last_replication)
	replicated := false

	// perform replication when we are leader and enough time has elapsed
	if r.state == leader && min_replication_interval < elapsedTime {
		for _, id := range r.node.nodeIds {
			if id == r.node.nodeId {
				continue
			}

			// select next entry to send to a node
			ni := r.next_index[id]
			entries := r.log.fromIndex(ni)

			// replicate when there are entries to replicate and heartbeat interval has elapsed
			if 0 < len(entries) || heartbeat_interval < elapsedTime {
				logSafe(fmt.Sprintf("replicating %v to %s", entries, id))
				replicated = true

				prepare_entries := make([]interface{}, 0)
				for _, entry := range entries {
					prepare_entries = append(prepare_entries, map[string]interface{}{
						"term": entry.term,
						"op":   entry.op,
					})
				}

				req := map[string]interface{}{
					"type":           "append_entries",
					"term":           r.term,
					"leader_id":      r.node.nodeId,
					"prev_log_index": ni - 1,
					"prev_log_term":  r.log.getEntry(ni - 1).term,
					"entries":        prepare_entries,
					"leader_commit":  r.commit_index,
				}

				r.node.rpc(
					id,
					req,
					func(req Request) error {
						body := req.Body.(map[string]interface{})
						term := body["term"].(float64)

						r.maybeStepDown(term)
						if r.state == leader && term == r.term {
							r.resetStepDownDeadline()
							if body["success"].(bool) {
								// handling success replication to a node
								r.next_index[req.Src] = max(r.next_index[req.Src], (ni + len(entries)))
								r.match_index[req.Src] = max(r.match_index[req.Src], (ni + len(entries) - 1))

								// advance commit index
								r.advanceCommitIndex()
							} else {
								// if too many failed, next_index will point to out of bound index
								// MEGUMIN: need to determine the source of problem here
								r.next_index[req.Src]--
							}

							logSafe(fmt.Sprintf("Next index = %d for id = %s", r.next_index[req.Src], req.Src))
						}

						return nil
					},
				)
			}
		}

		if replicated {
			r.last_replication = time.Now()
		}
	}
}

func (r *Raft) appendEntries(req Request) error {
	logSafe(fmt.Sprintf("append entries: %+v", req))

	body := req.Body.(map[string]interface{})
	term := body["term"].(float64)
	prev_log_index := int(body["prev_log_index"].(float64))
	prev_log_term := body["prev_log_term"].(float64)
	entries := body["entries"].([]interface{})

	raft_lock.Lock()
	defer raft_lock.Unlock()

	r.maybeStepDown(term)

	res := map[string]interface{}{
		"type":    "append_entries_res",
		"term":    r.term,
		"success": false,
	}

	// reject msg from older leader
	if term < r.term {
		r.node.reply(req, res)
		return newRPCError(36).Error("")
	}

	r.leader = body["leader_id"].(string)
	r.resetElectionDeadline()

	if prev_log_index <= 0 {
		err := newRPCError(39)
		err.LogError("prev log index must be greater than 0")
		return err.Error("")
	}
	e := r.log.getEntry(prev_log_index)

	if e.term != prev_log_term {
		r.node.reply(req, res)
		errLog := fmt.Sprintf("e.term = %f, prev_log_term = %f", e.term, prev_log_term)

		return newRPCError(40).Error(errLog)
	}

	// r.log.truncate(prev_log_index)
	convertedEntries := make([]Entry, len(entries))
	for i, entry := range entries {
		convertedEntries[i] = Entry{
			term: entry.(map[string]interface{})["term"].(float64),
			op:   entry.(map[string]interface{})["op"].(map[string]interface{}),
		}
	}
	r.log.appendEntry(convertedEntries)

	// advance commit pointer
	if r.commit_index < body["leader_commit"].(float64) {
		r.commit_index = math.Min(body["leader_commit"].(float64), float64(r.log.size()))
		r.advanceStateMachine()
	}

	// acknowledge
	res["success"] = true
	r.node.reply(req, res)

	return nil
}
