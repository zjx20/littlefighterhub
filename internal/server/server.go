package server

import (
	"bytes"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/zjx20/littlefighterhub/internal/room"
)

type Server struct {
	Rooms      map[int]*room.Room
	Clients    map[*websocket.Conn]*room.Player
	nextUserID int
	mu         sync.Mutex
	upgrader   websocket.Upgrader
}

func NewServer() *Server {
	s := &Server{
		Rooms:      make(map[int]*room.Room),
		Clients:    make(map[*websocket.Conn]*room.Player),
		nextUserID: 1,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true // Allow all connections
			},
		},
	}
	for i := 1; i <= 8; i++ {
		s.Rooms[i] = room.NewRoom(i)
	}
	return s
}

func (s *Server) NextUserID() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := s.nextUserID
	s.nextUserID++
	return id
}

func (s *Server) HandleConnections(w http.ResponseWriter, r *http.Request) {
	log.Printf("Handle new connection from %s, requested host: %s\n", r.RemoteAddr, r.Host)
	ws, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("Upgrade error:", err)
		return
	}
	defer ws.Close()

	player := &room.Player{
		ID:   s.NextUserID(),
		Conn: ws,
		IP:   ws.RemoteAddr(),
	}
	s.addClient(player)
	defer s.removeClient(player)

	log.Printf("Client connected: ID %d, IP %s\n", player.ID, player.IP)

	// Send YOUR_ID message
	yourIDMsg := []byte(fmt.Sprintf("YOUR_ID\n%d\n200\n-999\n-999\n-999", player.ID))
	if err := ws.WriteMessage(websocket.TextMessage, yourIDMsg); err != nil {
		log.Println("write:", err)
		return
	}

	for {
		_, msg, err := ws.ReadMessage()
		if err != nil {
			log.Printf("Client %d disconnected: %v\n", player.ID, err)
			break
		}
		s.handleMessage(player, msg)
	}
}

func (s *Server) addClient(player *room.Player) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Clients[player.Conn] = player
}

func (s *Server) removeClient(player *room.Player) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var playerRoom *room.Room
	for _, r := range s.Rooms {
		if _, ok := r.Players[player.ID]; ok {
			playerRoom = r
			break
		}
	}

	if playerRoom != nil {
		playerRoom.Mu.Lock()
		playerRoom.RemovePlayer(player.ID)
		log.Printf("Player %d removed from room %d", player.ID, playerRoom.ID)

		// Broadcast "left the Room" message
		chatMsg := []byte(fmt.Sprintf("CHAT\n%d\n%s\nleft the Room.", player.ID, player.Name))
		for _, p := range playerRoom.Players {
			if err := p.Conn.WriteMessage(websocket.TextMessage, chatMsg); err != nil {
				log.Printf("Error broadcasting to player %d: %v", p.ID, err)
			}
		}
		s.broadcastPlayerList(playerRoom)
		playerRoom.Mu.Unlock()
	}

	delete(s.Clients, player.Conn)
	log.Printf("Client %d removed.\n", player.ID)
}

func (s *Server) handleMessage(player *room.Player, msg []byte) {
	parts := strings.Split(string(msg), "\n")
	command := parts[0]

	if command != "FRAME" {
		log.Printf("Received from %d: %s\n", player.ID, string(msg))
	}

	switch command {
	case "LIST":
		s.handleList(player)
	case "JOIN":
		s.handleJoin(player, parts)
	case "LEAVE":
		s.handleLeave(player, parts)
	case "START":
		s.handleStart(player)
	case "CHAT":
		s.handleChat(player, parts)
	case "FRAME":
		s.handleFrame(player, msg)
	case "ADMIN":
		s.handleAdmin(player)
	case "CHANGE_LATENCY":
		s.handleChangeLatency(player, parts)
	case "AWAY":
		s.handleAway(player, msg)
	case "UPDATE_CONTROL_NAMES":
		s.handleUpdateControlNames(player, msg)
	default:
		log.Printf("Unknown command from player %d: %s\n", player.ID, command)
	}
}

func (s *Server) handleList(player *room.Player) {
	var b bytes.Buffer
	b.WriteString("LIST\n\n")

	for i := 1; i <= 8; i++ {
		r := s.Rooms[i]
		r.Mu.Lock()

		playerNames := []string{}
		for _, p := range r.Players {
			playerNames = append(playerNames, p.Name)
		}

		b.WriteString("¶\n")
		b.WriteString(fmt.Sprintf("Room\n%d\n%s\n%d\n%d\n%d\n%s\n",
			r.ID,
			r.State,
			r.Latency,
			time.Since(r.Time).Milliseconds(),
			len(r.Players),
			strings.Join(playerNames, ", "),
		))
		r.Mu.Unlock()
	}

	if err := player.Conn.WriteMessage(websocket.TextMessage, b.Bytes()); err != nil {
		log.Println("write:", err)
	}
}

func (s *Server) handleJoin(player *room.Player, parts []string) {
	if len(parts) < 8 {
		log.Printf("Invalid JOIN command from player %d", player.ID)
		return
	}

	roomID, err := strconv.Atoi(parts[1])
	if err != nil || roomID < 1 || roomID > 8 {
		log.Printf("Invalid room ID from player %d: %s", player.ID, parts[1])
		return
	}

	log.Printf("Player %d is trying to join room %d", player.ID, roomID)
	roomToJoin := s.Rooms[roomID]
	roomToJoin.Mu.Lock()
	defer roomToJoin.Mu.Unlock()

	if roomToJoin.State == "STARTED" {
		// TODO: Handle joining a started room
		log.Printf("Player %d tried to join a started room %d", player.ID, roomID)
		return
	}

	if len(roomToJoin.Players) >= 8 {
		// TODO: Handle full room
		log.Printf("Room %d is full", roomID)
		return
	}

	player.Name = parts[2]
	player.P1 = parts[3]
	player.P2 = parts[4]
	player.P3 = parts[5]
	player.P4 = parts[6]
	player.Achievements = parts[7]

	roomToJoin.AddPlayer(player)
	log.Printf("Player %d (%s) joined room %d", player.ID, player.Name, roomID)

	s.broadcastPlayerList(roomToJoin)
}

func (s *Server) broadcastPlayerList(r *room.Room) {
	var b bytes.Buffer
	b.WriteString(fmt.Sprintf("PLAYER_LIST\n%d\n%d\n", r.ID, r.Latency))

	for _, p := range r.Players {
		b.WriteString(fmt.Sprintf("¶\n%d\n%s\n%s\n%s\n%s\n%s\n%s\n",
			p.ID,
			p.Name,
			p.P1,
			p.P2,
			p.P3,
			p.P4,
			p.Achievements,
		))
	}

	for _, p := range r.Players {
		if err := p.Conn.WriteMessage(websocket.TextMessage, b.Bytes()); err != nil {
			log.Printf("Error broadcasting to player %d: %v", p.ID, err)
		}
	}
}

func (s *Server) handleLeave(player *room.Player, parts []string) {
	if len(parts) < 2 {
		log.Printf("Invalid LEAVE command from player %d", player.ID)
		return
	}

	roomID, err := strconv.Atoi(parts[1])
	if err != nil || roomID < 1 || roomID > 8 {
		log.Printf("Invalid room ID from player %d: %s", player.ID, parts[1])
		return
	}

	roomToLeave := s.Rooms[roomID]
	roomToLeave.Mu.Lock()
	defer roomToLeave.Mu.Unlock()

	if _, ok := roomToLeave.Players[player.ID]; !ok {
		log.Printf("Player %d is not in room %d", player.ID, roomID)
		return
	}

	roomToLeave.RemovePlayer(player.ID)
	log.Printf("Player %d left room %d", player.ID, roomID)

	leftRoomMsg := []byte(fmt.Sprintf("LEFT_ROOM\n%d", roomID))
	if err := player.Conn.WriteMessage(websocket.TextMessage, leftRoomMsg); err != nil {
		log.Printf("Error send LEFT_ROOM message to player %d: %v", player.ID, err)
	}

	s.broadcastPlayerList(roomToLeave)
}

func (s *Server) handleStart(player *room.Player) {
	var playerRoom *room.Room
	for _, r := range s.Rooms {
		r.Mu.Lock()
		if _, ok := r.Players[player.ID]; ok {
			playerRoom = r
		}
		r.Mu.Unlock()
		if playerRoom != nil {
			break
		}
	}

	if playerRoom == nil {
		log.Printf("Player %d is not in any room", player.ID)
		return
	}

	playerRoom.Mu.Lock()
	defer playerRoom.Mu.Unlock()

	playerRoom.State = "STARTED"
	playerRoom.IsSynchronizing = true
	playerRoom.SyncFrameBuffer = make(map[int][][]byte)
	for _, p := range playerRoom.Players {
		playerRoom.SyncFrameBuffer[p.ID] = make([][]byte, 0)
	}
	log.Printf("Room %d started by player %d, synchronizing...", playerRoom.ID, player.ID)

	// Broadcast ROOM_NOW_STARTED message
	startMsg := []byte(fmt.Sprintf("ROOM_NOW_STARTED\n%d\n%d", playerRoom.ID, time.Since(playerRoom.Time).Milliseconds()))
	for _, p := range playerRoom.Players {
		if err := p.Conn.WriteMessage(websocket.TextMessage, startMsg); err != nil {
			log.Printf("Error broadcasting to player %d: %v", p.ID, err)
		}
	}
}

func (s *Server) handleChat(player *room.Player, parts []string) {
	if len(parts) < 2 {
		log.Printf("Invalid CHAT command from player %d", player.ID)
		return
	}

	var playerRoom *room.Room
	for _, r := range s.Rooms {
		r.Mu.Lock()
		if _, ok := r.Players[player.ID]; ok {
			playerRoom = r
		}
		r.Mu.Unlock()
		if playerRoom != nil {
			break
		}
	}

	if playerRoom == nil {
		log.Printf("Player %d is not in any room", player.ID)
		return
	}

	playerRoom.Mu.Lock()
	defer playerRoom.Mu.Unlock()

	chatMsg := []byte(fmt.Sprintf("CHAT\n%d\n%s\n%s", player.ID, player.Name, parts[1]))
	for _, p := range playerRoom.Players {
		if err := p.Conn.WriteMessage(websocket.TextMessage, chatMsg); err != nil {
			log.Printf("Error broadcasting to player %d: %v", p.ID, err)
		}
	}
}

func (s *Server) handleFrame(player *room.Player, msg []byte) {
	var playerRoom *room.Room
	for _, r := range s.Rooms {
		// A quick check without lock
		if _, ok := r.Players[player.ID]; ok {
			playerRoom = r
			break
		}
	}

	if playerRoom == nil {
		// Player not in any room, might be a leftover message.
		return
	}

	playerRoom.Mu.Lock()
	defer playerRoom.Mu.Unlock()

	if !playerRoom.IsSynchronizing {
		// Regular frame forwarding
		s.broadcastFrame(playerRoom, player.ID, msg)
		return
	}

	// Synchronization logic
	if _, ok := playerRoom.SyncFrameBuffer[player.ID]; ok {
		playerRoom.SyncFrameBuffer[player.ID] = append(playerRoom.SyncFrameBuffer[player.ID], msg)
	}

	// Check if all players have sent enough frames
	allReady := true
	if len(playerRoom.Players) < 2 { // No need to sync for single player
		allReady = true
	} else {
		for _, p := range playerRoom.Players {
			if len(playerRoom.SyncFrameBuffer[p.ID]) < playerRoom.Latency {
				allReady = false
				break
			}
		}
	}

	if allReady {
		log.Printf("Room %d synchronized. Releasing frame buffer.", playerRoom.ID)
		playerRoom.IsSynchronizing = false

		// Release all buffered frames in a round-robin fashion to ensure fairness
		log.Printf("Broadcasting buffered frames for room %d in round-robin order.", playerRoom.ID)
		for i := 0; i < playerRoom.Latency; i++ {
			for _, p := range playerRoom.Players {
				// Ensure the player and their frame buffer for this index exist
				if frames, ok := playerRoom.SyncFrameBuffer[p.ID]; ok && i < len(frames) {
					s.broadcastFrame(playerRoom, p.ID, frames[i])
				}
			}
		}
		// Clear the buffer
		playerRoom.SyncFrameBuffer = nil
	}
}

func (s *Server) broadcastFrame(r *room.Room, senderID int, msg []byte) {
	for _, p := range r.Players {
		if p.ID != senderID {
			if err := p.Conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				log.Printf("Error broadcasting frame to player %d: %v", p.ID, err)
			}
		}
	}
}

func (s *Server) handleAdmin(player *room.Player) {
	log.Printf("Admin connected: %d", player.ID)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		// Send STATS
		statsMsg := []byte("STATS 0 0")
		if err := player.Conn.WriteMessage(websocket.TextMessage, statsMsg); err != nil {
			log.Printf("Error sending STATS to admin %d: %v", player.ID, err)
			return
		}

		// Send ROOM_LIST
		var b bytes.Buffer
		b.WriteString("ROOM_LIST\n")
		for i := 1; i <= 8; i++ {
			r := s.Rooms[i]
			r.Mu.Lock()
			var playersInfo []string
			for _, p := range r.Players {
				playersInfo = append(playersInfo, fmt.Sprintf("{Name: %s, ID: %d, IP: %s}", p.Name, p.ID, p.IP.String()))
			}
			b.WriteString(fmt.Sprintf("Room %d [%s] %d %d %s\n",
				r.ID,
				r.State,
				r.Latency,
				time.Since(r.Time).Milliseconds(),
				strings.Join(playersInfo, ", "),
			))
			r.Mu.Unlock()
		}
		if err := player.Conn.WriteMessage(websocket.TextMessage, b.Bytes()); err != nil {
			log.Printf("Error sending ROOM_LIST to admin %d: %v", player.ID, err)
			return
		}
	}
}

func (s *Server) handleChangeLatency(player *room.Player, parts []string) {
	if len(parts) < 2 {
		log.Printf("Invalid CHANGE_LATENCY command from player %d", player.ID)
		return
	}

	latency, err := strconv.Atoi(parts[1])
	if err != nil {
		log.Printf("Invalid latency from player %d: %s", player.ID, parts[1])
		return
	}

	var playerRoom *room.Room
	for _, r := range s.Rooms {
		r.Mu.Lock()
		if _, ok := r.Players[player.ID]; ok {
			playerRoom = r
		}
		r.Mu.Unlock()
		if playerRoom != nil {
			break
		}
	}

	if playerRoom == nil {
		log.Printf("Player %d is not in any room", player.ID)
		return
	}

	playerRoom.Mu.Lock()
	defer playerRoom.Mu.Unlock()

	playerRoom.Latency = latency
	log.Printf("Room %d latency changed to %d by player %d", playerRoom.ID, latency, player.ID)

	s.broadcastPlayerList(playerRoom)
}

func (s *Server) handleAway(player *room.Player, msg []byte) {
	s.broadcastToOthers(player, msg)
}

func (s *Server) handleUpdateControlNames(player *room.Player, msg []byte) {
	s.broadcastToOthers(player, msg)
}

func (s *Server) broadcastToOthers(player *room.Player, msg []byte) {
	var playerRoom *room.Room
	for _, r := range s.Rooms {
		r.Mu.Lock()
		if _, ok := r.Players[player.ID]; ok {
			playerRoom = r
		}
		r.Mu.Unlock()
		if playerRoom != nil {
			break
		}
	}

	if playerRoom == nil {
		log.Printf("Player %d is not in any room", player.ID)
		return
	}

	playerRoom.Mu.Lock()
	defer playerRoom.Mu.Unlock()

	s.broadcastFrame(playerRoom, player.ID, msg)
}
