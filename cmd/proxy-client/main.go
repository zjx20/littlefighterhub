package main

import (
	"encoding/json"
	"flag"
	"log"
	"net"
	"net/url"
	"sync"

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
	serverAddr := flag.String("server", "localhost:28080", "Proxy server address")
	gameAddr := flag.String("game", "localhost:8080", "Game server address (for host mode)")
	localAddr := flag.String("local", "localhost:8081", "Local address for game client to connect (for peer mode)")
	flag.Parse()

	log.Printf("Starting proxy client in %s mode", *mode)

	if *mode == "host" {
		serverURL := url.URL{Scheme: "ws", Host: *serverAddr, Path: "/ws-host"}
		runHostMode(serverURL, *gameAddr)
	} else {
		serverURL := url.URL{Scheme: "ws", Host: *serverAddr, Path: "/ws-peer"}
		runPeerMode(serverURL, *localAddr)
	}
}

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
		_, msg, err := controlConn.ReadMessage()
		if err != nil {
			log.Printf("Control connection closed: %v", err)
			return
		}

		var message Message
		if err := json.Unmarshal(msg, &message); err != nil {
			log.Printf("Error unmarshaling control message: %v", err)
			continue
		}

		if message.Type == "new_peer" {
			var payload NewPeerPayload
			if err := json.Unmarshal(message.Payload, &payload); err != nil {
				log.Printf("Error unmarshaling new_peer payload: %v", err)
				continue
			}
			log.Printf("Received notification for new peer: %s", payload.PeerID)
			go handlePeerForHost(u, gameAddr, payload.PeerID)
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
		msgType, msg, err := src.ReadMessage()
		if err != nil {
			dst.Close()
			break
		}
		if msgType == websocket.BinaryMessage {
			if _, err := dst.Write(msg); err != nil {
				break
			}
		}
	}
}
