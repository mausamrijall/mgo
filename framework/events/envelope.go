package events

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"time"
)

// Envelope is a CloudEvents-shaped wrapper (subset of the CloudEvents 1.0
// spec) used for queued and outbox delivery. Data holds the raw
// event-specific JSON.
type Envelope struct {
	ID          string          `json:"id"`
	Source      string          `json:"source"`
	SpecVersion string          `json:"specversion"`
	Type        string          `json:"type"`
	Time        time.Time       `json:"time"`
	DataContent string          `json:"datacontenttype"`
	Data        json.RawMessage `json:"data"`
}

// Source is the CloudEvents source attribute stamped on envelopes.
// Applications may override it before dispatching.
var Source = "mgo"

// NewEnvelope wraps event data under the given type name.
func NewEnvelope(eventType string, data any) Envelope {
	raw, _ := json.Marshal(data)
	var id [16]byte
	rand.Read(id[:])
	return Envelope{
		ID:          hex.EncodeToString(id[:]),
		Source:      Source,
		SpecVersion: "1.0",
		Type:        eventType,
		Time:        time.Now().UTC(),
		DataContent: "application/json",
		Data:        raw,
	}
}
