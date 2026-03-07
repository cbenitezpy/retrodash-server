package stream

import (
	"log"
	"sync"

	"github.com/cbenitezpy-ueno/retrodash-server/pkg/types"
)

// Broadcaster manages multiple stream clients and broadcasts frames.
type Broadcaster struct {
	mu      sync.RWMutex
	clients map[string]*Client
	encoder *Encoder
}

// NewBroadcaster creates a new frame broadcaster.
func NewBroadcaster(encoder *Encoder) *Broadcaster {
	return &Broadcaster{
		clients: make(map[string]*Client),
		encoder: encoder,
	}
}

// Subscribe adds a new client to receive frames.
func (b *Broadcaster) Subscribe(client *Client) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.clients[client.ID] = client
	log.Printf("Client %s subscribed (quality: %s)", client.ID, client.Quality)
}

// Unsubscribe removes a client from the broadcaster.
func (b *Broadcaster) Unsubscribe(clientID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if client, ok := b.clients[clientID]; ok {
		client.Close()
		delete(b.clients, clientID)
		log.Printf("Client %s unsubscribed", clientID)
	}
}

// Broadcast sends a frame to all subscribed clients.
// Uses non-blocking sends to avoid slow clients blocking others.
func (b *Broadcaster) Broadcast(frame *Frame) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	for _, client := range b.clients {
		// Non-blocking send - drop frame if client is slow
		select {
		case client.FrameChan <- frame:
			// Frame sent successfully
		default:
			// Channel full, client is slow - drop frame
			client.RecordFrameDropped()
		}
	}
}

// BroadcastWithQuality sends quality-appropriate frames to clients.
// highFrame is for high quality clients, lowFrame is for low quality clients.
func (b *Broadcaster) BroadcastWithQuality(highFrame, lowFrame *Frame) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	for _, client := range b.clients {
		var frame *Frame
		if client.Quality == types.QualityLow {
			frame = lowFrame
		} else {
			frame = highFrame
		}

		select {
		case client.FrameChan <- frame:
			// Frame sent
		default:
			client.RecordFrameDropped()
		}
	}
}

// ActiveClients returns the number of connected clients.
func (b *Broadcaster) ActiveClients() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.clients)
}

// GetClient returns a client by ID.
func (b *Broadcaster) GetClient(id string) *Client {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.clients[id]
}

// Shutdown closes all client connections.
func (b *Broadcaster) Shutdown() {
	b.mu.Lock()
	defer b.mu.Unlock()
	for id, client := range b.clients {
		client.Close()
		delete(b.clients, id)
	}
	log.Println("Broadcaster shutdown, all clients disconnected")
}
