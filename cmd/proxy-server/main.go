package main

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// Message represents control messages between client and server.
type Message struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// NewPeerPayload is the payload for the "new_peer" message.
type NewPeerPayload struct {
	PeerID string `json:"peer_id"`
}

// Room holds the state for a single game room.
type Room struct {
	hostConn      *websocket.Conn
	peers         map[string]*websocket.Conn
	lock          sync.Mutex // Protects access to hostConn and peers map
	hostWriteLock sync.Mutex // Protects writes to hostConn
}

func NewRoom() *Room {
	return &Room{
		peers: make(map[string]*websocket.Conn),
	}
}

// ConnManager handles all rooms.
type ConnManager struct {
	rooms map[string]*Room
	lock  sync.Mutex
}

func NewConnManager() *ConnManager {
	return &ConnManager{
		rooms: make(map[string]*Room),
	}
}

// getOrCreateRoom finds a room by ID or creates a new one.
func (cm *ConnManager) getOrCreateRoom(roomID string) *Room {
	cm.lock.Lock()
	defer cm.lock.Unlock()
	if room, ok := cm.rooms[roomID]; ok {
		return room
	}
	log.Printf("Creating new room: %s", roomID)
	room := NewRoom()
	cm.rooms[roomID] = room
	return room
}

// removeRoom deletes a room if it's empty.
func (cm *ConnManager) removeRoom(roomID string) {
	cm.lock.Lock()
	defer cm.lock.Unlock()
	if room, ok := cm.rooms[roomID]; ok {
		room.lock.Lock()
		isHostPresent := room.hostConn != nil
		room.lock.Unlock()
		if !isHostPresent {
			log.Printf("Removing empty room: %s", roomID)
			delete(cm.rooms, roomID)
		}
	}
}

// handleWebSocket is the main entry point for all new host connections.
func (cm *ConnManager) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	roomID := r.URL.Query().Get("room")
	if roomID == "" {
		log.Println("Rejecting connection: missing room ID")
		http.Error(w, "Room ID is required", http.StatusBadRequest)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Upgrade error for room %s: %v", roomID, err)
		return
	}

	room := cm.getOrCreateRoom(roomID)

	// The first message determines the client's role.
	msgType, p, err := conn.ReadMessage()
	if err != nil {
		log.Printf("Room %s: Error reading role message: %v", roomID, err)
		conn.Close()
		return
	}

	if msgType != websocket.TextMessage {
		log.Printf("Room %s: First message must be a text message for role definition.", roomID)
		conn.Close()
		return
	}

	var msg Message
	if err := json.Unmarshal(p, &msg); err != nil {
		log.Printf("Room %s: Error unmarshaling role message: %v", roomID, err)
		conn.Close()
		return
	}

	if msg.Type == "register_host" {
		room.registerHost(conn, roomID, func() { cm.removeRoom(roomID) })
	} else if msg.Type == "data_conn" {
		var payload NewPeerPayload
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			log.Printf("Room %s: Error unmarshaling data_conn payload: %v", roomID, err)
			conn.Close()
			return
		}
		room.pairConnections(payload.PeerID, conn)
	} else {
		log.Printf("Room %s: Unknown role type: %s", roomID, msg.Type)
		conn.Close()
	}
}

func (room *Room) registerHost(conn *websocket.Conn, roomID string, onHostDisconnect func()) {
	room.lock.Lock()
	if room.hostConn != nil {
		room.lock.Unlock()
		log.Printf("Room %s: Host already registered. Rejecting new host.", roomID)
		conn.Close()
		return
	}
	room.hostConn = conn
	room.lock.Unlock()
	log.Printf("Room %s: Host registered successfully.", roomID)

	defer func() {
		room.lock.Lock()
		room.hostConn = nil
		// Close all associated peer connections
		for peerID, peerConn := range room.peers {
			log.Printf("Room %s: Closing peer connection %s due to host disconnect.", roomID, peerID)
			peerConn.Close()
		}
		room.peers = make(map[string]*websocket.Conn) // Clear the peers map
		room.lock.Unlock()
		log.Printf("Room %s: Host disconnected and room cleaned up.", roomID)
		onHostDisconnect()
	}()

	// Goroutine to send application-level pings
	go func() {
		ticker := time.NewTicker(pingPeriod)
		defer ticker.Stop()
		pingMsg, _ := json.Marshal(Message{Type: "ping"})

		for {
			// Check if host is still connected before proceeding
			room.lock.Lock()
			hostExists := room.hostConn != nil
			room.lock.Unlock()
			if !hostExists {
				return
			}

			<-ticker.C

			// Use the dedicated write lock to send the ping
			room.hostWriteLock.Lock()
			conn.SetWriteDeadline(time.Now().Add(writeWait))
			err := conn.WriteMessage(websocket.TextMessage, pingMsg)
			room.hostWriteLock.Unlock()

			if err != nil {
				log.Printf("Room %s: Failed to send app-level ping to host, assuming it's dead.", roomID)
				conn.Close() // This will trigger the defer in registerHost and clean up the room
				return
			}
		}
	}()

	for {
		conn.SetReadDeadline(time.Now().Add(pongWait))
		msgType, p, err := conn.ReadMessage()
		if err != nil {
			log.Printf("Room %s: Host control connection closed: %v", roomID, err)
			break
		}

		if msgType == websocket.TextMessage {
			var msg Message
			if err := json.Unmarshal(p, &msg); err == nil && msg.Type == "pong" {
				continue
			}
		}
		log.Printf("Room %s: Received unexpected message on host control channel: %s", roomID, p)
	}
}

func (room *Room) registerPeer(conn *websocket.Conn, roomID string) {
	peerID := "peer_" + time.Now().Format("20060102150405.000000")
	log.Printf("Room %s: Peer connected with ID: %s", roomID, peerID)

	room.lock.Lock()
	if room.hostConn == nil {
		room.lock.Unlock()
		log.Printf("Room %s: No host available. Rejecting peer.", roomID)
		conn.Close()
		return
	}

	room.peers[peerID] = conn
	hostConnForWrite := room.hostConn
	room.lock.Unlock() // Release lock before writing to network

	payload, _ := json.Marshal(NewPeerPayload{PeerID: peerID})
	msg, _ := json.Marshal(Message{Type: "new_peer", Payload: payload})

	// Use the dedicated write lock to notify the host
	room.hostWriteLock.Lock()
	err := hostConnForWrite.WriteMessage(websocket.TextMessage, msg)
	room.hostWriteLock.Unlock()

	if err != nil {
		log.Printf("Room %s: Failed to notify host about new peer %s: %v", roomID, peerID, err)
		conn.Close()
		// Re-acquire lock to safely remove peer from map
		room.lock.Lock()
		delete(room.peers, peerID)
		room.lock.Unlock()
	}
}

func (room *Room) pairConnections(peerID string, hostDataConn *websocket.Conn) {
	room.lock.Lock()
	peerConn, ok := room.peers[peerID]
	if !ok {
		room.lock.Unlock()
		log.Printf("Peer %s not found for pairing.", peerID)
		hostDataConn.Close()
		return
	}
	delete(room.peers, peerID)
	room.lock.Unlock()

	log.Printf("Pairing host data connection with peer %s", peerID)

	var hostWriteMutex, peerWriteMutex sync.Mutex
	go appPinger(hostDataConn, &hostWriteMutex)
	go appPinger(peerConn, &peerWriteMutex)
	go forward(hostDataConn, peerConn, "Host -> Peer ("+peerID+")", &peerWriteMutex)
	go forward(peerConn, hostDataConn, "Peer -> Host ("+peerID+")", &hostWriteMutex)
}

// handlePeer is the entry point for peer connections.
func (cm *ConnManager) handlePeer(w http.ResponseWriter, r *http.Request) {
	roomID := r.URL.Query().Get("room")
	if roomID == "" {
		log.Println("Rejecting peer connection: missing room ID")
		http.Error(w, "Room ID is required", http.StatusBadRequest)
		return
	}

	room := cm.getOrCreateRoom(roomID)
	if room == nil {
		log.Printf("Could not get or create room %s", roomID)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Upgrade error for peer in room %s: %v", roomID, err)
		return
	}
	room.registerPeer(conn, roomID)
}

func main() {
	manager := NewConnManager()
	// The host connects here to register and establish a data connection.
	http.HandleFunc("/ws-host", manager.handleWebSocket)
	// Peers connect here.
	http.HandleFunc("/ws-peer", manager.handlePeer)

	log.Println("Proxy server started on :28080")
	err := http.ListenAndServe(":28080", nil)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}

const (
	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second
	// Time allowed to read the next pong message from the peer.
	pongWait = 10 * time.Second
	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 5) / 10
)

// appPinger sends application-level pings on a connection.
func appPinger(conn *websocket.Conn, writeMutex *sync.Mutex) {
	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()
	pingMsg, _ := json.Marshal(Message{Type: "ping"})

	for range ticker.C {
		writeMutex.Lock()
		conn.SetWriteDeadline(time.Now().Add(writeWait))
		err := conn.WriteMessage(websocket.TextMessage, pingMsg)
		writeMutex.Unlock()
		if err != nil {
			log.Printf("Pinger: closing connection due to write error: %v", err)
			conn.Close()
			return
		}
	}
}

// forward only forwards binary messages from src to dst.
func forward(src, dst *websocket.Conn, direction string, writeMutex *sync.Mutex) {
	defer func() {
		src.Close()
		dst.Close()
		log.Printf("Stopped forwarding for direction: %s", direction)
	}()
	for {
		// Note: The read deadline is set by the client's pong response.
		// Here we just read. If the client doesn't respond to our pings, this read will time out.
		msgType, msg, err := src.ReadMessage()
		if err != nil {
			log.Printf("Forwarder read error on %s: %v", direction, err)
			break
		}
		// We only forward game data, which should be binary.
		// Text messages are for control (ping/pong), which are handled by the client, not forwarded.
		if msgType == websocket.BinaryMessage {
			writeMutex.Lock()
			dst.SetWriteDeadline(time.Now().Add(writeWait))
			err := dst.WriteMessage(msgType, msg)
			writeMutex.Unlock()
			if err != nil {
				log.Printf("Forwarder write error on %s: %v", direction, err)
				break
			}
		}
	}
}
