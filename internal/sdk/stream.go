package sdk

import (
	"context"
	"errors"
	"sync"
)

// ErrStreamClosed is returned when attempting to send on a closed stream.
var ErrStreamClosed = errors.New("message stream is closed")

// MessageStream enables multi-turn conversations with a Claude agent
// over the WebSocket SDK protocol. It acts as a channel-based async
// iterator — the gateway pushes prompts via Send(), and the agent
// handler pulls them via Next().
//
// Inspired by OpenClaw's MessageStream (TypeScript async iterable)
// that enables pushing follow-up messages into active Claude sessions
// without restarting.
//
// Usage (gateway side):
//
//	stream := NewMessageStream(8)
//	stream.Send("Analyze this code")          // Round 1 prompt
//	// ... wait for agent result ...
//	stream.Send("Now review agent B's output") // Round 2 prompt
//	// ... wait for agent result ...
//	stream.Close()                             // No more prompts
//
// Usage (agent handler side):
//
//	for {
//	    msg, ok := stream.Next(ctx)
//	    if !ok { break }
//	    // Send msg as user message over WebSocket to Claude
//	}
type MessageStream struct {
	ch     chan string
	done   chan struct{}
	once   sync.Once
	closed bool
	mu     sync.RWMutex
}

// NewMessageStream creates a buffered message stream.
// bufferSize controls how many prompts can be queued before Send blocks.
func NewMessageStream(bufferSize int) *MessageStream {
	if bufferSize <= 0 {
		bufferSize = 1
	}
	return &MessageStream{
		ch:   make(chan string, bufferSize),
		done: make(chan struct{}),
	}
}

// Send queues a prompt for the agent. Blocks if the buffer is full.
// Returns ErrStreamClosed if the stream has been closed.
func (s *MessageStream) Send(msg string) error {
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return ErrStreamClosed
	}
	s.mu.RUnlock()

	select {
	case s.ch <- msg:
		return nil
	case <-s.done:
		return ErrStreamClosed
	}
}

// SendContext queues a prompt with context cancellation support.
func (s *MessageStream) SendContext(ctx context.Context, msg string) error {
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return ErrStreamClosed
	}
	s.mu.RUnlock()

	select {
	case s.ch <- msg:
		return nil
	case <-s.done:
		return ErrStreamClosed
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Next blocks until the next prompt is available, the stream is closed,
// or the context is cancelled. Returns (message, true) on success, or
// ("", false) when the stream is done.
func (s *MessageStream) Next(ctx context.Context) (string, bool) {
	select {
	case msg, ok := <-s.ch:
		if !ok {
			return "", false
		}
		return msg, true
	case <-s.done:
		// Drain any remaining messages
		select {
		case msg, ok := <-s.ch:
			if ok {
				return msg, true
			}
		default:
		}
		return "", false
	case <-ctx.Done():
		return "", false
	}
}

// Close signals that no more messages will be sent. Safe to call
// multiple times. Any pending Next() calls will return ("", false).
func (s *MessageStream) Close() {
	s.once.Do(func() {
		s.mu.Lock()
		s.closed = true
		s.mu.Unlock()
		close(s.done)
	})
}

// IsClosed returns whether the stream has been closed.
func (s *MessageStream) IsClosed() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.closed
}

// Pending returns the number of messages queued but not yet consumed.
func (s *MessageStream) Pending() int {
	return len(s.ch)
}
