package main

import (
    "math/rand"
    "sync"
)

// A single connected user
type Client struct {
    hub      *Hub
    conn     interface{ WriteMessage(int, []byte) error }
    send     chan []byte   // messages queued to send to this client
    roomCode string
    username string
}

// A chat room
type Room struct {
    clients  map[*Client]bool
    messages []string       // message history
    mu       sync.Mutex
}

// The central coordinator
type Hub struct {
    rooms      map[string]*Room         // roomCode → Room
    register   chan *Client             // client wants to join
    unregister chan *Client             // client disconnecting
    broadcast  chan BroadcastMessage    // message to send to a room
    mu         sync.RWMutex
}

type BroadcastMessage struct {
    roomCode string
    data     []byte
}

func newHub() *Hub {
    return &Hub{
        rooms:      make(map[string]*Room),
        register:   make(chan *Client),
        unregister: make(chan *Client),
        broadcast:  make(chan BroadcastMessage),
    }
}

// generateCode creates a random 6-character uppercase room code
func generateCode() string {
    const letters = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
    b := make([]byte, 6)
    for i := range b {
        b[i] = letters[rand.Intn(len(letters))]
    }
    return string(b)
}

// run is the Hub's main loop — runs in its own goroutine
func (h *Hub) run() {
    for {
        select {

        case client := <-h.register:
            // Add client to their room
            h.mu.Lock()
            room, ok := h.rooms[client.roomCode]
            if !ok {
                room = &Room{clients: make(map[*Client]bool)}
                h.rooms[client.roomCode] = room
            }
            room.clients[client] = true
            h.mu.Unlock()

        case client := <-h.unregister:
            // Remove client from their room; keep the room alive so the
            // code stays valid even when all members have left.
            h.mu.Lock()
            if room, ok := h.rooms[client.roomCode]; ok {
                delete(room.clients, client)
                close(client.send)
            }
            h.mu.Unlock()

        case msg := <-h.broadcast:
            // Send message to every client in the room
            h.mu.RLock()
            if room, ok := h.rooms[msg.roomCode]; ok {
                for client := range room.clients {
                    select {
                    case client.send <- msg.data:
                    default:
                        // Client's send buffer is full — disconnect them
                        close(client.send)
                        delete(room.clients, client)
                    }
                }
            }
            h.mu.RUnlock()
        }
    }
}