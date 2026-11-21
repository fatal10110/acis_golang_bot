package botregistry

import "sync"

// mailbox carries the runs' reports to the supervisor goroutine. A run posts
// without waiting on the supervisor, so a report made while the bot holds its
// login gate turn never stalls the other bots' logins; the supervisor selects
// on ready and takes the reports in the order they were posted, so a run's
// learned character always comes before its end. It holds at most a few
// reports per live run: its bot, its character and its end.
type mailbox struct {
	mu    sync.Mutex
	queue []event
	// ready holds a signal once a report was posted since the last take.
	ready chan struct{}
}

func newMailbox() *mailbox { return &mailbox{ready: make(chan struct{}, 1)} }

// post queues e and signals ready; it never blocks.
func (m *mailbox) post(e event) {
	m.mu.Lock()
	m.queue = append(m.queue, e)
	m.mu.Unlock()
	select {
	case m.ready <- struct{}{}:
	default:
	}
}

// take returns the queued reports, oldest first, and empties the queue.
func (m *mailbox) take() []event {
	m.mu.Lock()
	defer m.mu.Unlock()
	q := m.queue
	m.queue = nil
	return q
}
