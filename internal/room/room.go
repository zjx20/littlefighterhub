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
	Latency int
	Time    time.Time
	Players map[int]*Player
	Mu      sync.Mutex
}

func NewRoom(id int) *Room {
	return &Room{
		ID:      id,
		State:   "VACANT",
		Latency: 3,
		Time:    time.Now(),
		Players: make(map[int]*Player),
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
