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
	// Keep the host control connection alive by reading from it.
	// This loop will break if the host disconnects.
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			log.Printf("Host control connection closed: %v", err)
			cm.connLock.Lock()
			cm.hostConn = nil
			cm.connLock.Unlock()
			break
		}
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
	go forward(hostDataConn, peerConn, "Host -> Peer ("+peerID+")")
	go forward(peerConn, hostDataConn, "Peer -> Host ("+peerID+")")
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

// forward copies messages from src to dst.
func forward(src, dst *websocket.Conn, direction string) {
	defer func() {
		src.Close()
		dst.Close()
		log.Printf("Stopped forwarding for direction: %s", direction)
	}()

	for {
		msgType, msg, err := src.ReadMessage()
		if err != nil {
			break
		}
		if err := dst.WriteMessage(msgType, msg); err != nil {
			break
		}
	}
}
