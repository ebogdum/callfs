package telemetry

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	LevelInfo            = "info"
	LevelWarn            = "warn"
	LevelError           = "error"
	sseHeartbeatInterval = 15 * time.Second
)

type Entity struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

type Event struct {
	ID         uint64         `json:"id"`
	At         time.Time      `json:"at"`
	Type       string         `json:"type"`
	Level      string         `json:"level"`
	Actor      *string        `json:"actor"`
	InstanceID string         `json:"instance_id"`
	Message    string         `json:"message"`
	Entities   []Entity       `json:"entities"`
	Fields     map[string]any `json:"fields"`
}

type EventQuery struct {
	Types []string
	Actor string
	Since time.Time
	Limit int
}

type EventBus struct {
	mu         sync.Mutex
	instanceID string
	nextID     uint64
	entries    *ring[Event]
	subs       map[*Subscription]struct{}
}

func NewEventBus(instanceID string, capacity int) (*EventBus, error) {
	entries, err := newRing[Event](capacity)
	if nil != err {
		return nil, err
	}
	return &EventBus{instanceID: instanceID, entries: entries, subs: make(map[*Subscription]struct{})}, nil
}

func (b *EventBus) Publish(event Event) error {
	if "" == event.Type || strings.ContainsAny(event.Type, " \t\r\n") {
		return fmt.Errorf("%w: type", ErrInvalidEvent)
	}
	if LevelInfo != event.Level && LevelWarn != event.Level && LevelError != event.Level {
		return fmt.Errorf("%w: level", ErrInvalidEvent)
	}
	b.record(event)
	return nil
}

func (b *EventBus) record(event Event) {
	if nil == event.Entities {
		event.Entities = []Entity{}
	}
	if nil == event.Fields {
		event.Fields = map[string]any{}
	}
	if event.At.IsZero() {
		event.At = time.Now().UTC()
	}
	event.At = event.At.UTC()
	b.mu.Lock()
	defer b.mu.Unlock()
	b.nextID++
	event.ID = b.nextID
	event.InstanceID = b.instanceID
	b.entries.add(event)
	if 0 < len(b.subs) {
		fanout(b.subs, Message{ID: event.ID, Name: event.Type, Data: event})
	}
}

func (b *EventBus) Query(query EventQuery) ([]Event, error) {
	limit, err := pageLimit(query.Limit)
	if nil != err {
		return nil, err
	}
	matchesType := typeMatcher(query.Types)
	events := make([]Event, 0, min(limit, b.entries.size()))
	b.entries.scan(func(event Event) bool {
		if !matchesType(event.Type) {
			return true
		}
		if "" != query.Actor && (nil == event.Actor || *event.Actor != query.Actor) {
			return true
		}
		if !query.Since.IsZero() && event.At.Before(query.Since) {
			return true
		}
		events = append(events, event)
		return len(events) < limit
	})
	for i, j := 0, len(events)-1; i < j; i, j = i+1, j-1 {
		events[i], events[j] = events[j], events[i]
	}
	return events, nil
}

func (b *EventBus) Subscribe(types []string, afterID uint64) *Subscription {
	matchesType := typeMatcher(types)
	accept := func(message Message) bool {
		return matchesType(message.Name)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	var replay []Message
	if 0 < afterID {
		b.entries.scan(func(event Event) bool {
			if event.ID <= afterID {
				return false
			}
			message := Message{ID: event.ID, Name: event.Type, Data: event}
			if accept(message) {
				replay = append(replay, message)
			}
			return len(replay) < replayLimit
		})
		reverseMessages(replay)
	}
	sub := newSubscription(accept, replay)
	sub.release = detachFunc(&b.mu, b.subs, sub)
	b.subs[sub] = struct{}{}
	return sub
}

func typeMatcher(types []string) func(string) bool {
	allowed := make(map[string]struct{}, len(types))
	for _, name := range types {
		allowed[name] = struct{}{}
	}
	return func(name string) bool {
		if 0 == len(allowed) {
			return true
		}
		_, ok := allowed[name]
		return ok
	}
}

func ParseLastEventID(r *http.Request) uint64 {
	id, err := strconv.ParseUint(strings.TrimSpace(r.Header.Get("Last-Event-ID")), 10, 64)
	if nil != err {
		return 0
	}
	return id
}

func ServeSSE(w http.ResponseWriter, r *http.Request, sub *Subscription) error {
	defer sub.Close()
	header := w.Header()
	header.Set("Content-Type", "text/event-stream")
	header.Set("Cache-Control", "no-cache")
	header.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	for _, message := range sub.Replay {
		if err := writeSSEMessage(w, message); nil != err {
			return err
		}
	}
	controller := http.NewResponseController(w)
	if err := controller.Flush(); nil != err {
		return err
	}
	heartbeat := time.NewTicker(sseHeartbeatInterval)
	defer heartbeat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return nil
		case <-heartbeat.C:
			if _, err := io.WriteString(w, ": heartbeat\n\n"); nil != err {
				return err
			}
			if err := controller.Flush(); nil != err {
				return err
			}
		case message, open := <-sub.C:
			if !open {
				return nil
			}
			if err := writeSSEMessage(w, message); nil != err {
				return err
			}
			if err := controller.Flush(); nil != err {
				return err
			}
		}
	}
}

func writeSSEMessage(w io.Writer, message Message) error {
	payload, err := json.Marshal(message.Data)
	if nil != err {
		return err
	}
	_, err = fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", message.ID, message.Name, payload)
	return err
}
