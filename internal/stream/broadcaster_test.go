package stream

import (
	"testing"
	"time"

	"github.com/cbenitezpy-ueno/retrodash-server/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewBroadcaster(t *testing.T) {
	enc := DefaultEncoder()
	b := NewBroadcaster(enc)
	require.NotNil(t, b)
	assert.Equal(t, 0, b.ActiveClients())
}

func TestBroadcaster_SubscribeUnsubscribe(t *testing.T) {
	b := NewBroadcaster(DefaultEncoder())

	client := NewClient("client1", types.QualityHigh, "127.0.0.1:1234", 2)
	b.Subscribe(client)

	assert.Equal(t, 1, b.ActiveClients())
	assert.NotNil(t, b.GetClient("client1"))

	b.Unsubscribe("client1")
	assert.Equal(t, 0, b.ActiveClients())
	assert.Nil(t, b.GetClient("client1"))
}

func TestBroadcaster_Broadcast(t *testing.T) {
	b := NewBroadcaster(DefaultEncoder())

	// Create clients
	client1 := NewClient("client1", types.QualityHigh, "127.0.0.1:1234", 2)
	client2 := NewClient("client2", types.QualityHigh, "127.0.0.1:5678", 2)
	b.Subscribe(client1)
	b.Subscribe(client2)

	// Broadcast a frame
	frame := &Frame{
		Data:      []byte("test frame"),
		Timestamp: time.Now(),
		Sequence:  1,
	}
	b.Broadcast(frame)

	// Both clients should receive the frame
	select {
	case received := <-client1.FrameChan:
		assert.Equal(t, frame, received)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("client1 did not receive frame")
	}

	select {
	case received := <-client2.FrameChan:
		assert.Equal(t, frame, received)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("client2 did not receive frame")
	}
}

func TestBroadcaster_BroadcastWithQuality(t *testing.T) {
	b := NewBroadcaster(DefaultEncoder())

	highClient := NewClient("high", types.QualityHigh, "127.0.0.1:1234", 2)
	lowClient := NewClient("low", types.QualityLow, "127.0.0.1:5678", 2)
	b.Subscribe(highClient)
	b.Subscribe(lowClient)

	highFrame := &Frame{Data: []byte("high quality"), Quality: 85}
	lowFrame := &Frame{Data: []byte("low quality"), Quality: 50}

	b.BroadcastWithQuality(highFrame, lowFrame)

	// High quality client should get high frame
	select {
	case received := <-highClient.FrameChan:
		assert.Equal(t, highFrame, received)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("high client did not receive frame")
	}

	// Low quality client should get low frame
	select {
	case received := <-lowClient.FrameChan:
		assert.Equal(t, lowFrame, received)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("low client did not receive frame")
	}
}

func TestBroadcaster_SlowClientDropsFrames(t *testing.T) {
	b := NewBroadcaster(DefaultEncoder())

	// Create client with buffer size 1
	client := NewClient("slow", types.QualityHigh, "127.0.0.1:1234", 1)
	b.Subscribe(client)

	// Send multiple frames without reading
	for i := 0; i < 5; i++ {
		frame := &Frame{
			Data:     []byte("frame"),
			Sequence: uint64(i),
		}
		b.Broadcast(frame)
	}

	// Client should have dropped frames
	sent, dropped, _ := client.Stats()
	assert.Equal(t, uint64(0), sent) // Not sent yet (not read from channel)
	assert.Greater(t, dropped, uint64(0))
}

func TestBroadcaster_Shutdown(t *testing.T) {
	b := NewBroadcaster(DefaultEncoder())

	client1 := NewClient("client1", types.QualityHigh, "127.0.0.1:1234", 2)
	client2 := NewClient("client2", types.QualityLow, "127.0.0.1:5678", 2)
	b.Subscribe(client1)
	b.Subscribe(client2)

	assert.Equal(t, 2, b.ActiveClients())

	b.Shutdown()

	assert.Equal(t, 0, b.ActiveClients())
}

func TestClient_Stats(t *testing.T) {
	client := NewClient("test", types.QualityHigh, "127.0.0.1:1234", 2)

	// Record some activity
	client.RecordFrameSent(1000)
	client.RecordFrameSent(2000)
	client.RecordFrameDropped()

	sent, dropped, bytes := client.Stats()
	assert.Equal(t, uint64(2), sent)
	assert.Equal(t, uint64(1), dropped)
	assert.Equal(t, uint64(3000), bytes)
}

func TestNewClient(t *testing.T) {
	client := NewClient("test-id", types.QualityLow, "192.168.1.1:8080", 5)

	assert.Equal(t, "test-id", client.ID)
	assert.Equal(t, types.QualityLow, client.Quality)
	assert.Equal(t, "192.168.1.1:8080", client.RemoteAddr())
	assert.NotNil(t, client.FrameChan)
	assert.False(t, client.ConnectedAt.IsZero())
}
