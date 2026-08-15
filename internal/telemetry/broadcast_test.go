package telemetry

import (
	"testing"
	"time"
)

func TestLogBroadcaster_PublishDeliversToSubscriber(t *testing.T) {
	b := NewLogBroadcaster()
	ch, unsubscribe := b.Subscribe("service:web")
	defer unsubscribe()

	entry := LogEntry{ResourceID: "service:web", Stream: "stdout", Message: "hello"}
	b.Publish(entry)

	select {
	case got := <-ch:
		if got.Message != "hello" || got.Stream != "stdout" {
			t.Errorf("got %+v, want %+v", got, entry)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for a published entry")
	}
}

func TestLogBroadcaster_PublishOnlyReachesMatchingResourceID(t *testing.T) {
	b := NewLogBroadcaster()
	webCh, unsubWeb := b.Subscribe("service:web")
	defer unsubWeb()
	workerCh, unsubWorker := b.Subscribe("service:worker")
	defer unsubWorker()

	b.Publish(LogEntry{ResourceID: "service:web", Stream: "stdout", Message: "for web"})

	select {
	case got := <-webCh:
		if got.Message != "for web" {
			t.Errorf("webCh got %+v, want message 'for web'", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for service:web's own entry")
	}

	select {
	case got := <-workerCh:
		t.Errorf("workerCh received %+v, want nothing (entry was for a different resource)", got)
	case <-time.After(200 * time.Millisecond):
		// expected: no cross-resource delivery
	}
}

func TestLogBroadcaster_MultipleSubscribersAllReceive(t *testing.T) {
	b := NewLogBroadcaster()
	ch1, unsub1 := b.Subscribe("service:web")
	defer unsub1()
	ch2, unsub2 := b.Subscribe("service:web")
	defer unsub2()

	b.Publish(LogEntry{ResourceID: "service:web", Message: "fan-out"})

	for i, ch := range []<-chan LogEntry{ch1, ch2} {
		select {
		case got := <-ch:
			if got.Message != "fan-out" {
				t.Errorf("subscriber %d got %+v, want message 'fan-out'", i, got)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("subscriber %d timed out waiting for the published entry", i)
		}
	}
}

func TestLogBroadcaster_PublishWithNoSubscribersDoesNotBlockOrPanic(_ *testing.T) {
	b := NewLogBroadcaster()
	b.Publish(LogEntry{ResourceID: "service:nobody-listening", Message: "into the void"})
}

func TestLogBroadcaster_UnsubscribeStopsDelivery(t *testing.T) {
	b := NewLogBroadcaster()
	ch, unsubscribe := b.Subscribe("service:web")
	unsubscribe()

	b.Publish(LogEntry{ResourceID: "service:web", Message: "after unsubscribe"})

	select {
	case got, open := <-ch:
		if open {
			t.Errorf("received %+v on an unsubscribed channel, want no delivery", got)
		}
		// A closed-channel zero-value read is also acceptable: unsubscribe
		// only removes the channel from the subscriber list, it doesn't
		// close it (Publish is the only writer and it's already been
		// told to stop), so open=false here would mean something else
		// closed it, which never happens in this test, but either
		// outcome proves no live value ever arrives.
	case <-time.After(200 * time.Millisecond):
		// No delivery within a reasonable window: the expected outcome.
	}
}

func TestLogBroadcaster_SlowSubscriberIsDroppedNotBlocked(t *testing.T) {
	b := NewLogBroadcaster()
	ch, unsubscribe := b.Subscribe("service:web")
	defer unsubscribe()

	// Fill the subscriber's buffer without ever reading from ch, then
	// publish one more: Publish must return promptly (not block on the
	// full channel) and the caller must not panic. This is the entire
	// point of LogBroadcaster's non-blocking send: a slow SSE writer
	// downstream must never be able to stall LogCollector.StreamOne,
	// which is also busy consuming Docker's own log stream.
	done := make(chan struct{})
	go func() {
		for i := 0; i < broadcastBufferSize+10; i++ {
			b.Publish(LogEntry{ResourceID: "service:web", Message: "flood"})
		}
		close(done)
	}()

	select {
	case <-done:
		// Publish returned for every call without blocking on the
		// unread, full channel: exactly the non-blocking guarantee this
		// test exists to prove.
	case <-time.After(2 * time.Second):
		t.Fatal("Publish blocked against a full, unread subscriber channel")
	}

	// The channel should still be readable and hold at most
	// broadcastBufferSize buffered entries (the flood beyond that was
	// dropped, not queued indefinitely).
	buffered := 0
	for {
		select {
		case <-ch:
			buffered++
		default:
			if buffered > broadcastBufferSize {
				t.Errorf("buffered %d entries, want at most %d (broadcastBufferSize)", buffered, broadcastBufferSize)
			}
			return
		}
	}
}

func TestLogBroadcaster_UnsubscribeLastSubscriberRemovesResourceEntry(t *testing.T) {
	// Not observable from the public API directly, so this exercises the
	// behavior indirectly: subscribe and unsubscribe the only listener
	// for a resource, then subscribe again and confirm a fresh Publish
	// still reaches the new subscriber. If unsubscribe's map cleanup
	// (see Subscribe's own doc comment on why it deletes empty entries)
	// were broken in a way that corrupted the map, this second
	// subscription would misbehave.
	b := NewLogBroadcaster()
	_, unsubscribe := b.Subscribe("service:web")
	unsubscribe()

	ch2, unsub2 := b.Subscribe("service:web")
	defer unsub2()
	b.Publish(LogEntry{ResourceID: "service:web", Message: "after resubscribe"})

	select {
	case got := <-ch2:
		if got.Message != "after resubscribe" {
			t.Errorf("got %+v, want message 'after resubscribe'", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for delivery to the resubscribed listener")
	}
}
