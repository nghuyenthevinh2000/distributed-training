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

type State int

const (
	FOLLOWER                 State = 0
	CANDIDATE                State = 1
	LEADER                   State = 2
	election_timeout               = 2 * time.Second
	heartbeat_interval             = 1 * time.Second
	min_replication_interval       = 300 * time.Millisecond
)

// raft needs better lock mechanism
// currently, lock mechanism is locking out the whole raft struct which is wasteful
// a more localized lock mechanism is needed
type Raft struct {
	node          *Node
	state_machine *Map
	state         *SafeState
	leader        *SafeString

	election_deadline  *SafeTime
	step_down_deadline *SafeTime
	last_replication   *SafeTime

	term      *SafeFloat64
	log       *Log
	voted_for *SafeString

	// leader state
	commit_index *SafeFloat64
	last_applied *SafeFloat64
	next_index   *SafeIndex
	match_index  *SafeIndex
}

func (r *Raft) init() {
	r.node = newNode()
	r.state_machine = &Map{}
	r.state_machine.init()
	r.state = newSafeState(FOLLOWER)
	r.leader = newSafeString("")

	r.election_deadline = newSafeTime(time.Now())
	r.step_down_deadline = newSafeTime(time.Now())
	r.last_replication = newSafeTime(time.Now())

	r.term = newSafeFloat64(0)
	r.log = &Log{}
	r.log.init(r.node)
	r.voted_for = newSafeString("")

	r.commit_index = newSafeFloat64(0)
	r.last_applied = newSafeFloat64(1)
	r.next_index = newSafeIndex()
	r.match_index = newSafeIndex()

	r.node.on("read", r.clientRequest)
	r.node.on("write", r.clientRequest)
	r.node.on("cas", r.clientRequest)
	r.node.on("request_vote", r.handleVote)
	r.node.on("append_entries", r.appendEntries)

	// set our node to be candidate if no authority from leader is detected for more than 1 seconds
	getInterval := func() time.Duration {
		return time.Duration((500 + rand.Intn(500)) * int(time.Millisecond))
	}

	getMinReplicationInterval := func() time.Duration {
		return min_replication_interval
	}

	r.node.every(getInterval, r.checkAuthority)
	r.node.every(getInterval, r.checkStepDown)
	r.node.every(getMinReplicationInterval, r.replicateLog)
}

func (r *Raft) clientRequest(req Request) error {
	logSafe(fmt.Sprintf("handling client request: %v", req.Body))

	op := req.Body.(map[string]interface{})
	op["src"] = req.Src

	msg_id := op["msg_id"].(float64)

	if r.state.get() != LEADER {
		if r.leader.get() == "" {
			err := newRPCError(38)
			err.LogError("")

			return err.Error("")
		}

		// logSafe(fmt.Sprintf("not leader, forwarding request: %v to leader %s", op, leader))

		// if not leader, forward request to leader
		r.node.rpc(r.leader.get(), op, func(leaderReq Request) error {
			// extract result from leader, then send to original requester
			body := leaderReq.Body.(map[string]interface{})

			// logSafe(fmt.Sprintf("got result from leader: %v", body))
			// set msg id of original request
			op["msg_id"] = msg_id
			logSafe(fmt.Sprintf("current request: %v", req.Body))

			r.node.reply(req, body)
			return nil
		})

		return nil
	}

	// log is appended independently from replication, this allows more parrelism
	r.log.appendEntry([]Entry{
		{term: r.term.get(), op: op},
	})

	return nil
}

// this function is thread safe
func (r *Raft) becomeCandidate() {
	r.state.set(CANDIDATE)
	r.advanceTerm(r.term.get() + 1)
	r.voted_for.set(r.node.nodeId)
	r.leader.set("")
	r.resetElectionDeadline()
	r.resetStepDownDeadline()
	// logSafe("became candidate")
	r.requestVotes()
}

// this function is thread safe
func (r *Raft) becomeFollower() {
	r.state.set(FOLLOWER)

	// reset index maps to empty since these belong to leader only
	r.match_index = newSafeIndex()
	r.next_index = newSafeIndex()

	r.leader.set("")
	r.resetElectionDeadline()
	// logSafe(fmt.Sprintf("became follower for term %f", r.term))
}

// this function is thread safe
func (r *Raft) becomeLeader() {
	if r.state.get() != CANDIDATE {
		newRPCError(37).LogError("")
		return
	}

	r.state.set(LEADER)
	r.leader.set("")

	for _, id := range r.node.nodeIds {
		if id == r.node.nodeId {
			continue
		}
		r.next_index.set(id, r.log.size()+1)
		r.match_index.set(id, 0)
	}

	r.resetStepDownDeadline()
	// logSafe(fmt.Sprintf("became leader for term %f", r.term))
}

// election deadline will range from [2;3] seconds. This function is thread safe
func (r *Raft) resetElectionDeadline() {
	rand_time := time.Duration(rand.Intn(1000) * int(time.Millisecond))
	r.election_deadline.set(time.Now().Add(election_timeout + rand_time))
}

// this function is thread safe
func (r *Raft) resetStepDownDeadline() {
	r.step_down_deadline.set(time.Now().Add(election_timeout))
}

// this function is thread safe
func (r *Raft) checkAuthority() {
	// logSafe("Checking authority")

	if r.election_deadline.before(time.Now()) {
		if r.state.get() != LEADER {
			r.becomeCandidate()
		} else {
			r.resetElectionDeadline()
		}
	}
}

// this function is thread safe
func (r *Raft) checkStepDown() {
	// logSafe("checking step down")

	if r.state.get() == LEADER && r.step_down_deadline.before(time.Now()) {
		// logSafe("Stepping down as leader")
		r.becomeFollower()
	}
}

// new term must always be greater than current term. This function is thread safe
func (r *Raft) advanceTerm(term float64) {
	if r.term.get() > term {
		newRPCError(36).LogError("")
		return
	}

	r.term.set(term)

	// reset voted for because leader is chosen
	r.voted_for.set("")
}

// this function is thread safe
func (r *Raft) maybeStepDown(remote_term float64) {
	if r.term.get() < remote_term {
		// logSafe(fmt.Sprintf("remote term %f is greater than local term %f", remote_term, r.term))
		r.advanceTerm(remote_term)
		r.becomeFollower()
	}
}

// this function is thread safe
func (r *Raft) requestVotes() {
	// logSafe("Requesting votes")

	current_term := r.term.get()
	voteLock := sync.Mutex{}
	votes := map[string]bool{}

	// construct request vote rpc body
	resBody := map[string]interface{}{}
	resBody["type"] = "request_vote"
	resBody["term"] = r.term.get()
	resBody["candidate_id"] = r.node.nodeId
	resBody["last_log_index"] = r.log.size()
	resBody["last_log_term"] = r.log.lastEntry().term

	// send request vote rpc to all other nodes in the cluster
	r.node.brpc(resBody, func(req Request) error {
		// logSafe(fmt.Sprintf("got vote from %s", req.Src))

		r.resetStepDownDeadline()
		reqBody := req.Body.(map[string]interface{})
		term := reqBody["term"].(float64)
		r.maybeStepDown(term)
		if r.state.get() == CANDIDATE && r.term.get() == current_term && r.term.get() == term && reqBody["vote_granted"].(bool) {
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

// this function is thread safe
func (r *Raft) handleVote(req Request) error {
	// logSafe("handling vote")

	body := req.Body.(map[string]interface{})
	reqTerm := body["term"].(float64)
	lastLogTerm := body["last_log_term"].(float64)
	lastLogIndex := int(body["last_log_index"].(float64))

	r.maybeStepDown(reqTerm)

	grant := false
	if reqTerm < r.term.get() {
		// logSafe(fmt.Sprintf("request term: %f is smaller than current local term: %f, not granting vote", reqTerm, r.term))
	} else if r.voted_for.get() != "" {
		// logSafe(fmt.Sprintf("already voted for %s, not granting vote", r.voted_for))
	} else if lastLogTerm < r.log.lastEntry().term {
		// logSafe(fmt.Sprintf("request last log term: %f is smaller than local last log term: %f, not granting vote", lastLogTerm, r.log.lastEntry().term))
	} else if lastLogTerm == r.log.lastEntry().term && lastLogIndex < r.log.size() {
		// logSafe(fmt.Sprintf("logs are both at same term %f, but request last log index: %d is smaller than local last log index: %d, not granting vote", lastLogTerm, lastLogIndex, r.log.size()))
	} else {
		candidateId := body["candidate_id"].(string)
		// logSafe(fmt.Sprintf("granting vote to %s", candidateId))
		grant = true
		r.voted_for.set(candidateId)
		r.resetElectionDeadline()
	}

	r.node.reply(req, map[string]interface{}{
		"type":         "request_vote_res",
		"term":         r.term.get(),
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
	r.match_index.set(r.node.nodeId, r.log.size())
}

// this is thread safe
func (r *Raft) advanceCommitIndex() {
	if r.state.get() == LEADER {
		n := r.median(r.match_index.keys())

		if r.commit_index.get() < float64(n) && r.log.getEntry(n).term == r.term.get() {
			// logSafe(fmt.Sprintf(" commit index now= %v \n", n))
			r.commit_index.set(float64(n))
		}
	}

	r.advanceStateMachine()
}

// this will return last applied index
func (r *Raft) advanceStateMachine() {
	// benchmark advance state machine
	// startTime := time.Now()
	// logSafe("start advance state machine")

	for r.last_applied.get() < r.commit_index.get() {
		r.last_applied.set(r.last_applied.get() + 1)
		op := r.log.getEntry(int(r.last_applied.get())).op

		res, err := r.state_machine.apply(op)

		if err != nil {
			// logSafe(fmt.Sprintf("Error applying state machine: %s", err))
			continue
		}

		// logSafe(fmt.Sprintf("state machine result is = %v", res))

		if r.state.get() == LEADER {
			req := Request{
				Src:  op["src"].(string),
				Body: op,
			}
			r.node.reply(req, res)
		}
	}

	// logSafe(fmt.Sprintf("Done advance state machine in %d ms", time.Since(startTime).Microseconds()))
}

// this is thread safe
func (r *Raft) replicateLog() {
	// logSafe("replicating log")

	elapsedTime := time.Since(r.last_replication.get())
	replicated := false

	// perform replication when we are leader and enough time has elapsed
	// shorter log batching to meet 1s read constraint
	if r.state.get() == LEADER && min_replication_interval < elapsedTime {
		for _, id := range r.node.nodeIds {
			if id == r.node.nodeId {
				continue
			}

			// select next entry to send to a node
			ni := r.next_index.get(id)
			entries := r.log.fromIndex(ni)

			// replicate when there are entries to replicate or heartbeat interval has elapsed
			if 0 < len(entries) || heartbeat_interval < elapsedTime {
				// logSafe(fmt.Sprintf("replicating %v to %s", entries, id))
				replicated = true

				prepare_entries := make([]interface{}, 0)
				for _, entry := range entries {
					prepare_entries = append(prepare_entries, map[string]interface{}{
						"term": entry.term,
						"op":   entry.op,
					})
				}

				// send a batch of ops to other followers
				req := map[string]interface{}{
					"type":           "append_entries",
					"term":           r.term.get(),
					"leader_id":      r.node.nodeId,
					"prev_log_index": ni - 1,
					"prev_log_term":  r.log.getEntry(ni - 1).term,
					"entries":        prepare_entries,
					"leader_commit":  r.commit_index.get(),
				}

				r.node.rpc(
					id,
					req,
					func(req Request) error {
						body := req.Body.(map[string]interface{})
						term := body["term"].(float64)

						r.maybeStepDown(term)
						if r.state.get() == LEADER && term == r.term.get() {
							r.resetStepDownDeadline()
							// since test cases only cover 3 nodes, one more success means majority
							if body["success"].(bool) {
								// handling success replication to a node
								next_index := r.next_index.get(req.Src)
								match_index := r.match_index.get(req.Src)
								r.next_index.set(req.Src, max(next_index, (ni+len(entries))))
								r.match_index.set(req.Src, max(match_index, (ni+len(entries)-1)))

								// advance commit index
								r.advanceCommitIndex()
							} else {
								// if too many failed, next_index will point to out of bound index
								// MEGUMIN: need to determine the source of problem here
								value := r.next_index.get(req.Src) - 1
								r.next_index.set(req.Src, value)
							}

							// logSafe(fmt.Sprintf("Next index = %d for id = %s", r.next_index[req.Src], req.Src))
						}

						return nil
					},
				)
			}
		}

		if replicated {
			r.last_replication.set(time.Now())
		}
	}
}

func (r *Raft) appendEntries(req Request) error {
	body := req.Body.(map[string]interface{})
	term := body["term"].(float64)
	prev_log_index := int(body["prev_log_index"].(float64))
	prev_log_term := body["prev_log_term"].(float64)
	entries := body["entries"].([]interface{})

	// logSafe("handling append entries")

	r.maybeStepDown(term)

	res := map[string]interface{}{
		"type":    "append_entries_res",
		"term":    r.term.get(),
		"success": false,
	}

	// reject msg from older leader
	if term < r.term.get() {
		r.node.reply(req, res)
		return newRPCError(36).Error("")
	}

	r.leader.set(body["leader_id"].(string))
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

	convertedEntries := make([]Entry, len(entries))
	for i, entry := range entries {
		convertedEntries[i] = Entry{
			term: entry.(map[string]interface{})["term"].(float64),
			op:   entry.(map[string]interface{})["op"].(map[string]interface{}),
		}
	}
	r.log.appendEntry(convertedEntries)

	// advance commit pointer
	if r.commit_index.get() < body["leader_commit"].(float64) {
		r.commit_index.set(math.Min(body["leader_commit"].(float64), float64(r.log.size())))
		r.advanceStateMachine()
	}

	// acknowledge
	// logSafe("acknowledging append entries")
	res["success"] = true
	r.node.reply(req, res)

	return nil
}
