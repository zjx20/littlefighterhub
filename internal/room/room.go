package room

import (
	"net"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type Player struct {
	ID           int
	Name         string
	Conn         *websocket.Conn
	IP           net.Addr
	Achievements string
	P1           string
	P2           string
	P3           string
	P4           string
}

type Room struct {
	ID      int
	State   string // VACANT, LOBBY, STARTED
	Players map[int]*Player
	Time    time.Time
	Latency int
	Mu      sync.Mutex

	// For synchronizing frames at the beginning of a match
	IsSynchronizing bool
	SyncFrameBuffer map[int][][]byte
}

func NewRoom(id int) *Room {
	return &Room{
		ID:              id,
		State:           "VACANT",
		Players:         make(map[int]*Player),
		Time:            time.Now(),
		Latency:         3,
		IsSynchronizing: false,
		SyncFrameBuffer: make(map[int][][]byte),
	}
}

func (r *Room) AddPlayer(player *Player) {
	r.Players[player.ID] = player
	if len(r.Players) > 0 && r.State == "VACANT" {
		r.State = "LOBBY"
	}
}

func (r *Room) RemovePlayer(playerID int) {
	delete(r.Players, playerID)
	if len(r.Players) == 0 {
		r.State = "VACANT"
	}
}
