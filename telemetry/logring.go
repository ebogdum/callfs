package telemetry

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap/zapcore"
)

const logEventName = "log"

type LogEntry struct {
	ID        uint64         `json:"id"`
	At        time.Time      `json:"at"`
	Level     string         `json:"level"`
	Message   string         `json:"message"`
	Logger    string         `json:"logger"`
	Fields    map[string]any `json:"fields"`
	RequestID *string        `json:"request_id"`
	severity  zapcore.Level
}

type LogQuery struct {
	MinLevel  string
	Text      string
	RequestID string
	Since     time.Time
	Limit     int
	Cursor    string
}

type LogPage struct {
	Items      []LogEntry `json:"items"`
	NextCursor *string    `json:"next_cursor"`
}

type LogRing struct {
	mu      sync.Mutex
	nextID  uint64
	entries *ring[LogEntry]
	subs    map[*Subscription]struct{}
}

func NewLogRing(capacity int) (*LogRing, error) {
	entries, err := newRing[LogEntry](capacity)
	if nil != err {
		return nil, err
	}
	return &LogRing{entries: entries, subs: make(map[*Subscription]struct{})}, nil
}

func (l *LogRing) Core() zapcore.Core {
	return &logCore{ring: l}
}

func (l *LogRing) CoreAt(enabler zapcore.LevelEnabler) zapcore.Core {
	return &logCore{ring: l, enabler: enabler}
}

func (l *LogRing) append(entry zapcore.Entry, base, extra []zapcore.Field) {
	var values map[string]any
	if 0 == len(base)+len(extra) {
		values = map[string]any{}
	} else {
		encoder := zapcore.NewMapObjectEncoder()
		for i := range base {
			base[i].AddTo(encoder)
		}
		for i := range extra {
			extra[i].AddTo(encoder)
		}
		values = encoder.Fields
	}
	var requestID *string
	if value, ok := values["request_id"].(string); ok && "" != value {
		requestID = &value
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.nextID++
	record := LogEntry{
		ID:        l.nextID,
		At:        entry.Time.UTC(),
		Level:     entry.Level.String(),
		Message:   entry.Message,
		Logger:    entry.LoggerName,
		Fields:    values,
		RequestID: requestID,
		severity:  entry.Level,
	}
	l.entries.add(record)
	if 0 < len(l.subs) {
		fanout(l.subs, Message{ID: record.ID, Name: logEventName, Data: record})
	}
}

func (l *LogRing) Query(query LogQuery) (LogPage, error) {
	limit, err := pageLimit(query.Limit)
	if nil != err {
		return LogPage{}, err
	}
	before, err := parseCursor(query.Cursor)
	if nil != err {
		return LogPage{}, err
	}
	matches, err := query.compile()
	if nil != err {
		return LogPage{}, err
	}
	items := make([]LogEntry, 0, min(limit, l.entries.size()))
	hasMore := false
	l.entries.scan(func(entry LogEntry) bool {
		if 0 < before && entry.ID >= before {
			return true
		}
		if !matches(entry) {
			return true
		}
		if len(items) == limit {
			hasMore = true
			return false
		}
		items = append(items, entry)
		return true
	})
	var next *string
	if hasMore {
		cursor := encodeCursor(items[len(items)-1].ID)
		next = &cursor
	}
	return LogPage{Items: items, NextCursor: next}, nil
}

func (l *LogRing) Subscribe(query LogQuery, afterID uint64) (*Subscription, error) {
	matches, err := query.compile()
	if nil != err {
		return nil, err
	}
	accept := func(message Message) bool {
		entry, ok := message.Data.(LogEntry)
		return ok && matches(entry)
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	var replay []Message
	if 0 < afterID {
		l.entries.scan(func(entry LogEntry) bool {
			if entry.ID <= afterID {
				return false
			}
			message := Message{ID: entry.ID, Name: logEventName, Data: entry}
			if accept(message) {
				replay = append(replay, message)
			}
			return len(replay) < replayLimit
		})
		reverseMessages(replay)
	}
	sub := newSubscription(accept, replay)
	sub.release = detachFunc(&l.mu, l.subs, sub)
	l.subs[sub] = struct{}{}
	return sub, nil
}

func (q LogQuery) compile() (func(LogEntry) bool, error) {
	minLevel := zapcore.DebugLevel
	if "" != q.MinLevel {
		if err := minLevel.UnmarshalText([]byte(q.MinLevel)); nil != err {
			return nil, fmt.Errorf("%w: level", ErrInvalidQuery)
		}
	}
	text := strings.ToLower(q.Text)
	return func(entry LogEntry) bool {
		if entry.severity < minLevel {
			return false
		}
		if !q.Since.IsZero() && entry.At.Before(q.Since) {
			return false
		}
		if "" != q.RequestID && (nil == entry.RequestID || *entry.RequestID != q.RequestID) {
			return false
		}
		return "" == text || strings.Contains(strings.ToLower(entry.Message), text)
	}, nil
}

type logCore struct {
	ring    *LogRing
	fields  []zapcore.Field
	enabler zapcore.LevelEnabler
}

func (c *logCore) Enabled(level zapcore.Level) bool {
	return nil == c.enabler || c.enabler.Enabled(level)
}

func (c *logCore) With(fields []zapcore.Field) zapcore.Core {
	return &logCore{ring: c.ring, fields: c.combine(fields), enabler: c.enabler}
}

func (c *logCore) Check(entry zapcore.Entry, checked *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if !c.Enabled(entry.Level) {
		return checked
	}
	return checked.AddCore(entry, c)
}

func (c *logCore) Write(entry zapcore.Entry, fields []zapcore.Field) error {
	c.ring.append(entry, c.fields, fields)
	return nil
}

func (c *logCore) Sync() error {
	return nil
}

func (c *logCore) combine(fields []zapcore.Field) []zapcore.Field {
	combined := make([]zapcore.Field, 0, len(c.fields)+len(fields))
	combined = append(combined, c.fields...)
	return append(combined, fields...)
}
