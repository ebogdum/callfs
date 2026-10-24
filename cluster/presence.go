package cluster

import (
	"slices"
	"time"
)

type Presence interface {
	Self() Member
	Members() []Member
	Alive(id string) bool
	Watch(func(Event)) func()
}

type Member struct {
	ID          string    `json:"id"`
	ServerName  string    `json:"server_name"`
	APIEndpoint string    `json:"api_endpoint"`
	Version     string    `json:"version"`
	Zone        string    `json:"zone,omitempty"`
	StartedAt   time.Time `json:"started_at"`
	Caps        []string  `json:"caps,omitempty"`
	Seen        time.Time `json:"seen"`
}

func (m Member) HasCap(capability string) bool {
	return slices.Contains(m.Caps, capability)
}

type EventKind int

const (
	MemberUp EventKind = iota + 1
	MemberUpdated
	MemberDown
)

func (k EventKind) String() string {
	switch k {
	case MemberUp:
		return "up"
	case MemberUpdated:
		return "updated"
	case MemberDown:
		return "down"
	default:
		return "unknown"
	}
}

type Event struct {
	Kind   EventKind
	Member Member
}
