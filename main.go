package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
)

// upgrader converts a normal HTTP connection into a WebSocket connection
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins (fine for local dev)
	},
}

func main() {
	hub := newHub()
	go hub.run() // Start the hub in its own goroutine

	r := mux.NewRouter()

	// Serve static files (HTML, CSS, JS) from the ./static folder
	r.PathPrefix("/static/").Handler(
		http.StripPrefix("/static/", http.FileServer(http.Dir("./static"))),
	)

	// API: Create a new room, returns the room code
	r.HandleFunc("/api/create-room", func(w http.ResponseWriter, r *http.Request) {
		hub.mu.Lock()
		defer hub.mu.Unlock()

		// Generate a unique code
		code := generateCode()
		for _, exists := hub.rooms[code]; exists; _, exists = hub.rooms[code] {
			code = generateCode()
		}
		// Create the room now so the code is reserved
		hub.rooms[code] = &Room{clients: make(map[*Client]bool)}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"code": code})
	}).Methods("POST")

	// API: Check if a room exists
	r.HandleFunc("/api/room/{code}", func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		code := vars["code"]

		hub.mu.RLock()
		_, exists := hub.rooms[code]
		hub.mu.RUnlock()

		if !exists {
			http.Error(w, "Room not found", http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	}).Methods("GET")

	// WebSocket endpoint — this is where chat happens
	r.HandleFunc("/ws/{code}", func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		code := vars["code"]
		username := r.URL.Query().Get("username")
		if username == "" {
			username = "Anonymous"
		}

		// Check room exists
		hub.mu.RLock()
		_, exists := hub.rooms[code]
		hub.mu.RUnlock()
		if !exists {
			http.Error(w, "Room not found", http.StatusNotFound)
			return
		}

		// Upgrade HTTP → WebSocket
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Println("Upgrade error:", err)
			return
		}

		client := &Client{
			hub:      hub,
			send:     make(chan []byte, 256),
			roomCode: code,
			username: username,
		}

		hub.register <- client

		// Broadcast a "joined" notification
		joinMsg := fmt.Sprintf(`{"type":"system","text":"%s joined the room"}`, username)
		hub.broadcast <- BroadcastMessage{roomCode: code, data: []byte(joinMsg)}

		// Start reading and writing in separate goroutines
		go clientWriter(conn, client)
		clientReader(conn, client, hub) // blocks until client disconnects
	})

	// Serve the main HTML page for any /room/* path
	r.PathPrefix("/room/").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "./static/index.html")
	})

	// Home page
	r.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "./static/index.html")
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	fmt.Println("Server starting on port", port)
	log.Fatal(http.ListenAndServe(":"+port, r))
}

// clientWriter pumps messages from the client's send channel to the WebSocket
func clientWriter(conn *websocket.Conn, client *Client) {
	defer conn.Close()
	for msg := range client.send {
		if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
			break
		}
	}
}

// clientReader reads messages from the WebSocket and broadcasts them
func clientReader(conn *websocket.Conn, client *Client, hub *Hub) {
	defer func() {
		hub.unregister <- client
		conn.Close()

		// Broadcast "left" notification
		leaveMsg := fmt.Sprintf(`{"type":"system","text":"%s left the room"}`, client.username)
		hub.broadcast <- BroadcastMessage{roomCode: client.roomCode, data: []byte(leaveMsg)}
	}()

	for {
		_, msgBytes, err := conn.ReadMessage()
		if err != nil {
			break // Client disconnected
		}

		// Wrap in JSON with username
		out := fmt.Sprintf(`{"type":"message","username":"%s","text":%s}`,
			client.username, string(msgBytes))
		hub.broadcast <- BroadcastMessage{roomCode: client.roomCode, data: []byte(out)}
	}
}
