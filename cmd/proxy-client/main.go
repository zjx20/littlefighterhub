package main

import (
	"encoding/json"
	"flag"
	"log"
	"net"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Message represents control messages between client and server.
type Message struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// NewPeerPayload is the payload for the "new_peer" message.
type NewPeerPayload struct {
	PeerID string `json:"peer_id"`
}

func main() {
	mode := flag.String("mode", "peer", "Set mode to 'host' or 'peer'")
	serverAddr := flag.String("server", "localhost:28080", "Proxy server address (e.g., wss://your.server.com)")
	gameAddr := flag.String("game", "localhost:8080", "Game server address (for host mode)")
	localAddr := flag.String("local", "localhost:8081", "Local address for game client to connect (for peer mode)")
	flag.Parse()

	log.Printf("Starting proxy client in %s mode", *mode)

	// Parse the server address
	u, err := url.Parse(*serverAddr)
	if err != nil {
		log.Fatalf("Invalid server URL: %v", err)
	}

	// If scheme is missing, prepend ws:// and re-parse
	if u.Scheme == "" {
		u, err = url.Parse("ws://" + *serverAddr)
		if err != nil {
			log.Fatalf("Invalid server URL: %v", err)
		}
	}

	// Allow http/https as aliases for ws/wss
	if u.Scheme == "https" {
		u.Scheme = "wss"
	} else if u.Scheme == "http" {
		u.Scheme = "ws"
	}

	// Construct the final URL for the specific mode
	serverURL := *u // Make a copy
	if *mode == "host" {
		serverURL.Path = "/ws-host"
		runHostMode(serverURL, *gameAddr)
	} else {
		serverURL.Path = "/ws-peer"
		runPeerMode(serverURL, *localAddr)
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

// runHostMode runs the client that connects to the game server.
func runHostMode(u url.URL, gameAddr string) {
	log.Printf("Running in host mode. Control connection to %s", u.String())

	// Establish the main control connection
	controlConn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		log.Fatalf("Failed to establish control connection: %v", err)
	}
	defer controlConn.Close()

	// Register as host
	registerMsg, _ := json.Marshal(Message{Type: "register_host"})
	if err := controlConn.WriteMessage(websocket.TextMessage, registerMsg); err != nil {
		log.Fatalf("Failed to register as host: %v", err)
	}
	log.Println("Registered as host. Waiting for new peer notifications...")

	// Listen for new peer notifications
	for {
		controlConn.SetReadDeadline(time.Now().Add(pongWait))
		_, p, err := controlConn.ReadMessage()
		if err != nil {
			log.Printf("Control connection closed: %v", err)
			return
		}

		var msg Message
		if err := json.Unmarshal(p, &msg); err != nil {
			log.Printf("Error unmarshaling control message: %v", err)
			continue
		}

		switch msg.Type {
		case "new_peer":
			var payload NewPeerPayload
			if err := json.Unmarshal(msg.Payload, &payload); err != nil {
				log.Printf("Error unmarshaling new_peer payload: %v", err)
				continue
			}
			log.Printf("Received notification for new peer: %s", payload.PeerID)
			go handlePeerForHost(u, gameAddr, payload.PeerID)
		case "ping":
			// Respond to server's ping
			pongMsg, _ := json.Marshal(Message{Type: "pong"})
			controlConn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := controlConn.WriteMessage(websocket.TextMessage, pongMsg); err != nil {
				log.Printf("Error sending pong: %v", err)
				return
			}
		}
	}
}

// handlePeerForHost creates a new data connection for a specific peer.
func handlePeerForHost(u url.URL, gameAddr, peerID string) {
	log.Printf("[%s] Creating data channel...", peerID)
	// 1. Establish a new data websocket connection
	dataConn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		log.Printf("[%s] Failed to create data connection: %v", peerID, err)
		return
	}
	defer dataConn.Close()

	// 2. Send data_conn message to pair with peer
	payload, _ := json.Marshal(NewPeerPayload{PeerID: peerID})
	msg, _ := json.Marshal(Message{Type: "data_conn", Payload: payload})
	if err := dataConn.WriteMessage(websocket.TextMessage, msg); err != nil {
		log.Printf("[%s] Failed to send data_conn message: %v", peerID, err)
		return
	}

	// 3. Connect to the local game server
	gameConn, err := net.Dial("tcp", gameAddr)
	if err != nil {
		log.Printf("[%s] Failed to connect to local game server at %s: %v", peerID, gameAddr, err)
		return
	}
	defer gameConn.Close()

	log.Printf("[%s] Data channel established. Forwarding data.", peerID)
	// 4. Forward data
	var wg sync.WaitGroup
	wg.Add(2)
	go forwardToWS(gameConn, dataConn, &wg)
	go forwardToTCP(dataConn, gameConn, &wg)
	wg.Wait()
	log.Printf("[%s] Data channel closed.", peerID)
}

// runPeerMode runs the client that the game client connects to.
func runPeerMode(u url.URL, localAddr string) {
	log.Printf("Running in peer mode, listening for game client on %s", localAddr)
	listener, err := net.Listen("tcp", localAddr)
	if err != nil {
		log.Fatalf("Failed to listen on %s: %v", localAddr, err)
	}
	defer listener.Close()

	for {
		gameConn, err := listener.Accept()
		if err != nil {
			log.Printf("Failed to accept game connection: %v", err)
			continue
		}
		log.Printf("Accepted connection from game client: %s", gameConn.RemoteAddr())
		go handleGameConnectionForPeer(gameConn, u)
	}
}

func handleGameConnectionForPeer(gameConn net.Conn, u url.URL) {
	defer gameConn.Close()

	log.Printf("Connecting to proxy server %s", u.String())
	wsConn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		log.Printf("Failed to dial proxy server: %v", err)
		return
	}
	defer wsConn.Close()

	log.Printf("Successfully connected to proxy server.")

	var wg sync.WaitGroup
	wg.Add(2)
	go forwardToWS(gameConn, wsConn, &wg)
	go forwardToTCP(wsConn, gameConn, &wg)
	wg.Wait()
	log.Println("Connection closed.")
}

func forwardToWS(src net.Conn, dst *websocket.Conn, wg *sync.WaitGroup) {
	defer wg.Done()
	buf := make([]byte, 2048)
	for {
		n, err := src.Read(buf)
		if err != nil {
			dst.Close()
			break
		}
		if err := dst.WriteMessage(websocket.BinaryMessage, buf[:n]); err != nil {
			break
		}
	}
}

func forwardToTCP(src *websocket.Conn, dst net.Conn, wg *sync.WaitGroup) {
	defer wg.Done()
	for {
		src.SetReadDeadline(time.Now().Add(pongWait))
		msgType, p, err := src.ReadMessage()
		if err != nil {
			log.Printf("forwardToTCP: read error: %v", err)
			dst.Close()
			break
		}

		if msgType == websocket.BinaryMessage {
			if _, err := dst.Write(p); err != nil {
				log.Printf("forwardToTCP: write error: %v", err)
				break
			}
		} else if msgType == websocket.TextMessage {
			var msg Message
			if err := json.Unmarshal(p, &msg); err == nil && msg.Type == "ping" {
				pongMsg, _ := json.Marshal(Message{Type: "pong"})
				src.SetWriteDeadline(time.Now().Add(writeWait))
				if err := src.WriteMessage(websocket.TextMessage, pongMsg); err != nil {
					log.Printf("forwardToTCP: error sending pong: %v", err)
					break
				}
			}
		}
	}
}
