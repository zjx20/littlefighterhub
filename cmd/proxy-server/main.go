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

// ConnManager handles all connection logic.
type ConnManager struct {
	hostConn *websocket.Conn
	peers    map[string]*websocket.Conn
	connLock sync.Mutex
}

func NewConnManager() *ConnManager {
	return &ConnManager{
		peers: make(map[string]*websocket.Conn),
	}
}

// handleWebSocket is the main entry point for all new connections.
func (cm *ConnManager) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Upgrade error: %v", err)
		return
	}

	// The first message determines the client's role.
	msgType, p, err := conn.ReadMessage()
	if err != nil {
		log.Printf("Error reading role message: %v", err)
		conn.Close()
		return
	}

	if msgType != websocket.TextMessage {
		log.Println("First message must be a text message for role definition.")
		conn.Close()
		return
	}

	var msg Message
	if err := json.Unmarshal(p, &msg); err != nil {
		log.Printf("Error unmarshaling role message: %v", err)
		conn.Close()
		return
	}

	if msg.Type == "register_host" {
		cm.registerHost(conn)
	} else if msg.Type == "data_conn" {
		// This is a data connection from the host, for a specific peer.
		var payload NewPeerPayload
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			log.Printf("Error unmarshaling data_conn payload: %v", err)
			conn.Close()
			return
		}
		cm.pairConnections(payload.PeerID, conn)
	} else {
		log.Printf("Unknown role type: %s", msg.Type)
		conn.Close()
	}
}

func (cm *ConnManager) registerHost(conn *websocket.Conn) {
	cm.connLock.Lock()
	if cm.hostConn != nil {
		cm.connLock.Unlock()
		log.Println("Host already registered. Rejecting new host.")
		conn.Close()
		return
	}
	cm.hostConn = conn
	cm.connLock.Unlock()
	log.Println("Host registered successfully.")

	// Goroutine to send application-level pings
	go func() {
		ticker := time.NewTicker(pingPeriod)
		defer ticker.Stop()
		pingMsg, _ := json.Marshal(Message{Type: "ping"})

		for range ticker.C {
			conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := conn.WriteMessage(websocket.TextMessage, pingMsg); err != nil {
				log.Println("Failed to send app-level ping to host, assuming it's dead.")
				conn.Close()
				return
			}
		}
	}()

	// Keep the host control connection alive by reading messages.
	for {
		conn.SetReadDeadline(time.Now().Add(pongWait))
		msgType, p, err := conn.ReadMessage()
		if err != nil {
			log.Printf("Host control connection closed: %v", err)
			cm.connLock.Lock()
			cm.hostConn = nil
			cm.connLock.Unlock()
			break
		}

		if msgType == websocket.TextMessage {
			var msg Message
			if err := json.Unmarshal(p, &msg); err == nil && msg.Type == "pong" {
				// It's a pong, deadline is reset by the next loop iteration. Continue.
				continue
			}
		}
		// Any other message on the control channel is unexpected.
		log.Printf("Received unexpected message on host control channel: %s", p)
	}
}

// registerPeer is called when a peer connects.
func (cm *ConnManager) registerPeer(conn *websocket.Conn) {
	peerID := "peer_" + time.Now().Format("20060102150405.000000")
	log.Printf("Peer connected with ID: %s", peerID)

	cm.connLock.Lock()
	if cm.hostConn == nil {
		cm.connLock.Unlock()
		log.Println("No host available. Rejecting peer.")
		conn.Close()
		return
	}

	cm.peers[peerID] = conn

	payload, _ := json.Marshal(NewPeerPayload{PeerID: peerID})
	msg, _ := json.Marshal(Message{Type: "new_peer", Payload: payload})

	err := cm.hostConn.WriteMessage(websocket.TextMessage, msg)
	cm.connLock.Unlock()

	if err != nil {
		log.Printf("Failed to notify host about new peer %s: %v", peerID, err)
		conn.Close()
		cm.connLock.Lock()
		delete(cm.peers, peerID)
		cm.connLock.Unlock()
	}
}

func (cm *ConnManager) pairConnections(peerID string, hostDataConn *websocket.Conn) {
	cm.connLock.Lock()
	peerConn, ok := cm.peers[peerID]
	if !ok {
		cm.connLock.Unlock()
		log.Printf("Peer %s not found for pairing.", peerID)
		hostDataConn.Close()
		return
	}
	// Remove from pending peers map
	delete(cm.peers, peerID)
	cm.connLock.Unlock()

	log.Printf("Pairing host data connection with peer %s", peerID)

	var hostWriteMutex, peerWriteMutex sync.Mutex

	// Start application-level pinger for both data connections
	go appPinger(hostDataConn, &hostWriteMutex)
	go appPinger(peerConn, &peerWriteMutex)

	// Start forwarding binary data
	go forward(hostDataConn, peerConn, "Host -> Peer ("+peerID+")", &peerWriteMutex)
	go forward(peerConn, hostDataConn, "Peer -> Host ("+peerID+")", &hostWriteMutex)
}

// handlePeer is the entry point for peer connections.
func (cm *ConnManager) handlePeer(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Upgrade error for peer: %v", err)
		return
	}
	cm.registerPeer(conn)
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
