package sdk

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestMessageStreamSendAndNext(t *testing.T) {
	s := NewMessageStream(4)
	defer s.Close()

	if err := s.Send("hello"); err != nil {
		t.Fatalf("Send failed: %v", err)
	}
	if err := s.Send("world"); err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	ctx := context.Background()

	msg, ok := s.Next(ctx)
	if !ok || msg != "hello" {
		t.Errorf("Next() = (%q, %v), want (hello, true)", msg, ok)
	}

	msg, ok = s.Next(ctx)
	if !ok || msg != "world" {
		t.Errorf("Next() = (%q, %v), want (world, true)", msg, ok)
	}
}

func TestMessageStreamClose(t *testing.T) {
	s := NewMessageStream(4)

	if err := s.Send("before close"); err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	s.Close()

	// Should still drain buffered messages
	ctx := context.Background()
	msg, ok := s.Next(ctx)
	if !ok || msg != "before close" {
		t.Errorf("Next() after close should drain buffer, got (%q, %v)", msg, ok)
	}

	// Now should return false
	msg, ok = s.Next(ctx)
	if ok {
		t.Errorf("Next() after drain should return false, got (%q, true)", msg)
	}
}

func TestMessageStreamSendAfterClose(t *testing.T) {
	s := NewMessageStream(4)
	s.Close()

	err := s.Send("should fail")
	if err != ErrStreamClosed {
		t.Errorf("Send after Close() = %v, want ErrStreamClosed", err)
	}
}

func TestMessageStreamDoubleClose(t *testing.T) {
	s := NewMessageStream(4)
	s.Close()
	s.Close() // Should not panic
}

func TestMessageStreamIsClosed(t *testing.T) {
	s := NewMessageStream(4)

	if s.IsClosed() {
		t.Error("IsClosed() should be false before Close()")
	}

	s.Close()

	if !s.IsClosed() {
		t.Error("IsClosed() should be true after Close()")
	}
}

func TestMessageStreamPending(t *testing.T) {
	s := NewMessageStream(8)
	defer s.Close()

	if s.Pending() != 0 {
		t.Errorf("Pending() = %d, want 0", s.Pending())
	}

	s.Send("a")
	s.Send("b")
	s.Send("c")

	if s.Pending() != 3 {
		t.Errorf("Pending() = %d, want 3", s.Pending())
	}

	s.Next(context.Background())

	if s.Pending() != 2 {
		t.Errorf("Pending() = %d, want 2", s.Pending())
	}
}

func TestMessageStreamContextCancellation(t *testing.T) {
	s := NewMessageStream(4)
	defer s.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	msg, ok := s.Next(ctx)
	if ok {
		t.Errorf("Next() with cancelled context should return false, got (%q, true)", msg)
	}
}

func TestMessageStreamSendContextCancellation(t *testing.T) {
	// Buffer of 1, fill it up
	s := NewMessageStream(1)
	defer s.Close()

	s.Send("fills buffer")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	err := s.SendContext(ctx, "should timeout")
	if err == nil {
		t.Fatal("SendContext should fail when buffer full and context expires")
	}
}

func TestMessageStreamSendContextAfterClose(t *testing.T) {
	s := NewMessageStream(4)
	s.Close()

	err := s.SendContext(context.Background(), "should fail")
	if err != ErrStreamClosed {
		t.Errorf("SendContext after Close() = %v, want ErrStreamClosed", err)
	}
}

func TestMessageStreamNextBlocksUntilSend(t *testing.T) {
	s := NewMessageStream(4)
	defer s.Close()

	done := make(chan string)
	go func() {
		msg, ok := s.Next(context.Background())
		if ok {
			done <- msg
		} else {
			done <- ""
		}
	}()

	// Small delay to ensure Next() is blocking
	time.Sleep(20 * time.Millisecond)

	s.Send("delayed")

	select {
	case msg := <-done:
		if msg != "delayed" {
			t.Errorf("got %q, want delayed", msg)
		}
	case <-time.After(time.Second):
		t.Fatal("Next() did not unblock after Send()")
	}
}

func TestMessageStreamNextUnblocksOnClose(t *testing.T) {
	s := NewMessageStream(4)

	done := make(chan bool)
	go func() {
		_, ok := s.Next(context.Background())
		done <- ok
	}()

	time.Sleep(20 * time.Millisecond)
	s.Close()

	select {
	case ok := <-done:
		if ok {
			t.Error("Next() should return false after Close()")
		}
	case <-time.After(time.Second):
		t.Fatal("Next() did not unblock after Close()")
	}
}

func TestMessageStreamConcurrentSendNext(t *testing.T) {
	s := NewMessageStream(16)
	defer s.Close()

	n := 100
	ctx := context.Background()

	var wg sync.WaitGroup

	// Producer
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < n; i++ {
			s.Send("msg")
		}
	}()

	// Consumer
	received := 0
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < n; i++ {
			_, ok := s.Next(ctx)
			if ok {
				received++
			}
		}
	}()

	wg.Wait()

	if received != n {
		t.Errorf("received %d messages, want %d", received, n)
	}
}

func TestMessageStreamDefaultBufferSize(t *testing.T) {
	s := NewMessageStream(0) // Should default to 1
	defer s.Close()

	if cap(s.ch) != 1 {
		t.Errorf("buffer cap = %d, want 1 (default)", cap(s.ch))
	}

	s2 := NewMessageStream(-5) // Negative should also default to 1
	defer s2.Close()

	if cap(s2.ch) != 1 {
		t.Errorf("buffer cap = %d, want 1 (default)", cap(s2.ch))
	}
}
