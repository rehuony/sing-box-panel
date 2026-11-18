// SPDX-License-Identifier: GPL-3.0-or-later

package fleet

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"slices"
	"strings"
)

// ErrStatusCursorInvalid rejects a cursor that no fleet page returned.
var ErrStatusCursorInvalid = errors.New("the fleet status cursor is invalid")

// StatusFilter selects nodes of a fleet snapshot. Empty fields select every
// node.
type StatusFilter struct {
	// State is online or offline.
	State string
	Tag   string
	// Query is part of a node's name, hostname or public IP, ignoring case.
	Query string
}

// selects reports whether the filter selects node; query is the filter's
// Query in lower case.
func (filter StatusFilter) selects(query string, node Node) bool {
	if filter.State != "" && node.State != filter.State {
		return false
	}
	if filter.Tag != "" && !slices.Contains(node.Tags, filter.Tag) {
		return false
	}
	return query == "" || strings.Contains(strings.ToLower(node.Name), query) ||
		strings.Contains(strings.ToLower(node.Hostname), query) || strings.Contains(strings.ToLower(node.PublicIP), query)
}

// StatusPage is the encoded items of the nodes a filter selects, in name
// order.
type StatusPage struct {
	Items []json.RawMessage `json:"items"`
	// NextCursor continues with the next page; empty on the last page.
	NextCursor string `json:"next_cursor,omitempty"`
}

// Page returns at most limit of the nodes filter selects, all of them when
// limit is zero, after the node cursor names. Cursors hold a position in the
// name order, so a page continues correctly after nodes change.
func (snapshot FleetSnapshot) Page(filter StatusFilter, limit int, cursor string) (StatusPage, error) {
	var after *statusPosition
	if cursor != "" {
		position, err := parseStatusCursor(cursor)
		if err != nil {
			return StatusPage{}, err
		}
		after = &position
	}
	query := strings.ToLower(filter.Query)
	page := StatusPage{Items: []json.RawMessage{}}
	last := -1
	for index, item := range snapshot.Nodes {
		if after != nil && !after.precedes(item.Node) || !filter.selects(query, item.Node) {
			continue
		}
		if limit > 0 && len(page.Items) == limit {
			node := snapshot.Nodes[last].Node
			page.NextCursor = statusPosition{name: node.Name, id: node.ID}.cursor()
			break
		}
		page.Items = append(page.Items, snapshot.Encoded[index])
		last = index
	}
	return page, nil
}

// StatusFrame is one change to the fleet a stream client holds: with Reset,
// every selected node; otherwise the nodes whose items changed and the IDs of
// nodes deleted or no longer selected.
type StatusFrame struct {
	Reset   bool              `json:"reset"`
	Items   []json.RawMessage `json:"items"`
	Removed []string          `json:"removed"`
}

// StatusChanges follows what one stream client holds of the nodes a filter
// selects.
type StatusChanges struct {
	filter StatusFilter
	query  string
	sent   map[string]json.RawMessage
}

// NewStatusChanges follows a client that holds nothing yet.
func NewStatusChanges(filter StatusFilter) *StatusChanges {
	return &StatusChanges{filter: filter, query: strings.ToLower(filter.Query)}
}

// Next returns what the client needs to hold snapshot: every selected node
// the first time, then only changes. It reports false when nothing changed.
func (changes *StatusChanges) Next(snapshot FleetSnapshot) (StatusFrame, bool) {
	frame := StatusFrame{Reset: changes.sent == nil, Items: []json.RawMessage{}, Removed: []string{}}
	selected := make(map[string]json.RawMessage, len(changes.sent))
	for index, item := range snapshot.Nodes {
		if !changes.filter.selects(changes.query, item.Node) {
			continue
		}
		encoded := snapshot.Encoded[index]
		selected[item.Node.ID] = encoded
		if previous, held := changes.sent[item.Node.ID]; frame.Reset || !held || !bytes.Equal(previous, encoded) {
			frame.Items = append(frame.Items, encoded)
		}
	}
	for id := range changes.sent {
		if _, kept := selected[id]; !kept {
			frame.Removed = append(frame.Removed, id)
		}
	}
	slices.Sort(frame.Removed)
	changes.sent = selected
	return frame, frame.Reset || len(frame.Items) > 0 || len(frame.Removed) > 0
}

// statusPosition is a place in the fleet's order: names ignoring ASCII case,
// as the store's NOCASE collation compares them, then IDs.
type statusPosition struct {
	name, id string
}

// precedes reports whether node comes after the position.
func (position statusPosition) precedes(node Node) bool {
	if order := strings.Compare(foldASCII(node.Name), foldASCII(position.name)); order != 0 {
		return order > 0
	}
	return node.ID > position.id
}

// cursor encodes the position. Names hold no control characters, so NUL
// separates the name from the ID.
func (position statusPosition) cursor() string {
	return base64.RawURLEncoding.EncodeToString([]byte(position.name + "\x00" + position.id))
}

// ValidStatusCursor reports whether cursor is one a fleet page returns.
func ValidStatusCursor(cursor string) bool {
	_, err := parseStatusCursor(cursor)
	return err == nil
}

func parseStatusCursor(cursor string) (statusPosition, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return statusPosition{}, ErrStatusCursorInvalid
	}
	name, id, found := strings.Cut(string(decoded), "\x00")
	if !found || id == "" {
		return statusPosition{}, ErrStatusCursorInvalid
	}
	return statusPosition{name: name, id: id}, nil
}

func foldASCII(value string) string {
	return strings.Map(func(character rune) rune {
		if character >= 'A' && character <= 'Z' {
			return character + 'a' - 'A'
		}
		return character
	}, value)
}
