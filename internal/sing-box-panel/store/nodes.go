// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

var (
	ErrNodeNotFound      = errors.New("node not found")
	ErrNodeNameConflict  = errors.New("a node with this name already exists")
	ErrEnrollmentInvalid = errors.New("enrollment token is invalid, used or expired")
)

// Node is one registered node: its enrollment state, its identity key once
// enrolled, and what it last reported about itself.
type Node struct {
	ID                  string
	Name                string
	CreatedAt           time.Time
	PublicKey           []byte
	EnrolledAt          *time.Time
	EnrollmentExpiresAt *time.Time
	Hostname            string
	OS                  string
	Arch                string
	BuildVersion        string
	ProtocolMinor       int
	LastSeenAt          *time.Time
	// SessionID names the node's current session; empty while it has none.
	SessionID string
}

// NewNode is a node record with a pending enrollment token.
type NewNode struct {
	ID        string
	Name      string
	CreatedAt time.Time
	TokenHash [32]byte
	ExpiresAt time.Time
}

// NodeEnrollment is what a node presents when it enrolls.
type NodeEnrollment struct {
	PublicKey     []byte
	Hostname      string
	OS            string
	Arch          string
	BuildVersion  string
	ProtocolMinor int
	EnrolledAt    time.Time
}

// NodeSession is what a node reports when its session is authenticated.
type NodeSession struct {
	ID            string
	OS            string
	Arch          string
	BuildVersion  string
	ProtocolMinor int
	SeenAt        time.Time
}

const nodeColumns = `id, name, created_at, public_key, enrolled_at, enrollment_expires_at,
	hostname, os, arch, build_version, protocol_minor, last_seen_at, session_id`

// PanelID returns the panel's stable identity, recording candidate the first
// time it is asked.
func (s *Store) PanelID(ctx context.Context, candidate string) (string, error) {
	if _, err := s.db.ExecContext(ctx, `INSERT INTO fleet_identity(singleton, panel_id) VALUES(1, ?) ON CONFLICT(singleton) DO NOTHING`, candidate); err != nil {
		return "", err
	}
	var id string
	err := s.db.QueryRowContext(ctx, `SELECT panel_id FROM fleet_identity WHERE singleton = 1`).Scan(&id)
	return id, err
}

func (s *Store) CreateNode(ctx context.Context, input NewNode) (Node, error) {
	row := s.db.QueryRowContext(ctx, `INSERT INTO nodes(id, name, created_at, enrollment_token_hash, enrollment_expires_at)
		VALUES(?,?,?,?,?) RETURNING `+nodeColumns,
		input.ID, input.Name, formatTime(input.CreatedAt), input.TokenHash[:], formatTime(input.ExpiresAt))
	return scanNodeResult(row)
}

func (s *Store) Nodes(ctx context.Context) ([]Node, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+nodeColumns+` FROM nodes ORDER BY name COLLATE NOCASE, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	nodes := []Node{}
	for rows.Next() {
		node, err := scanNode(rows)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}
	return nodes, rows.Err()
}

func (s *Store) Node(ctx context.Context, id string) (Node, error) {
	return scanNodeResult(s.db.QueryRowContext(ctx, `SELECT `+nodeColumns+` FROM nodes WHERE id = ?`, id))
}

// NodeByName finds a node by its name, ignoring case.
func (s *Store) NodeByName(ctx context.Context, name string) (Node, error) {
	return scanNodeResult(s.db.QueryRowContext(ctx, `SELECT `+nodeColumns+` FROM nodes WHERE name = ?`, name))
}

func (s *Store) RenameNode(ctx context.Context, id, name string) (Node, error) {
	return scanNodeResult(s.db.QueryRowContext(ctx, `UPDATE nodes SET name = ? WHERE id = ? RETURNING `+nodeColumns, name, id))
}

func (s *Store) DeleteNode(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM nodes WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if rows, err := result.RowsAffected(); err != nil {
		return err
	} else if rows == 0 {
		return ErrNodeNotFound
	}
	return nil
}

// IssueNodeEnrollment replaces the node's pending enrollment token. The
// enrolled key stays valid until a node enrolls with the new token.
func (s *Store) IssueNodeEnrollment(ctx context.Context, id string, tokenHash [32]byte, expiresAt time.Time) (Node, error) {
	return scanNodeResult(s.db.QueryRowContext(ctx, `UPDATE nodes SET enrollment_token_hash = ?, enrollment_expires_at = ?
		WHERE id = ? RETURNING `+nodeColumns, tokenHash[:], formatTime(expiresAt), id))
}

// EnrollNode consumes an unexpired enrollment token and records the node's
// key, replacing any key it had.
func (s *Store) EnrollNode(ctx context.Context, tokenHash [32]byte, input NodeEnrollment) (Node, error) {
	node, err := scanNodeResult(s.db.QueryRowContext(ctx, `UPDATE nodes SET public_key = ?, enrolled_at = ?,
		enrollment_token_hash = NULL, enrollment_expires_at = NULL, session_id = NULL,
		hostname = ?, os = ?, arch = ?, build_version = ?, protocol_minor = ?
		WHERE enrollment_token_hash = ? AND enrollment_expires_at > ? RETURNING `+nodeColumns,
		input.PublicKey, formatTime(input.EnrolledAt), input.Hostname, input.OS, input.Arch, input.BuildVersion,
		input.ProtocolMinor, tokenHash[:], formatTime(input.EnrolledAt)))
	if errors.Is(err, ErrNodeNotFound) {
		return Node{}, ErrEnrollmentInvalid
	}
	return node, err
}

// RecordNodeSession makes input the current session of the node enrolled
// with publicKey. It fails with ErrNodeNotFound once the node was deleted or
// enrolled with another key.
func (s *Store) RecordNodeSession(ctx context.Context, id string, publicKey []byte, input NodeSession) error {
	return expectOneNode(s.db.ExecContext(ctx, `UPDATE nodes SET session_id = ?, os = ?, arch = ?, build_version = ?, protocol_minor = ?, last_seen_at = ?
		WHERE id = ? AND public_key = ?`, input.ID, input.OS, input.Arch, input.BuildVersion, input.ProtocolMinor, formatTime(input.SeenAt), id, publicKey))
}

// TouchNode records that the node enrolled with publicKey was seen at seenAt,
// with the same failure as RecordNodeSession.
func (s *Store) TouchNode(ctx context.Context, id string, publicKey []byte, seenAt time.Time) error {
	return expectOneNode(s.db.ExecContext(ctx, `UPDATE nodes SET last_seen_at = ? WHERE id = ? AND public_key = ?`, formatTime(seenAt), id, publicKey))
}

// EndNodeSession records the end of session sessionID if it is still the
// node's current session.
func (s *Store) EndNodeSession(ctx context.Context, id, sessionID string, seenAt time.Time) error {
	_, err := s.db.ExecContext(ctx, `UPDATE nodes SET session_id = NULL, last_seen_at = ? WHERE id = ? AND session_id = ?`, formatTime(seenAt), id, sessionID)
	return err
}

// ClearNodeSessions forgets sessions left by a panel process that stopped
// without ending them.
func (s *Store) ClearNodeSessions(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `UPDATE nodes SET session_id = NULL WHERE session_id IS NOT NULL`)
	return err
}

func expectOneNode(result sql.Result, err error) error {
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNodeNotFound
	}
	return nil
}

func scanNodeResult(row *sql.Row) (Node, error) {
	node, err := scanNode(row)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return Node{}, ErrNodeNotFound
	case err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed: nodes.name"):
		return Node{}, ErrNodeNameConflict
	}
	return node, err
}

func scanNode(row rowScanner) (Node, error) {
	var node Node
	var createdAt string
	var enrolledAt, expiresAt, lastSeenAt, sessionID sql.NullString
	if err := row.Scan(&node.ID, &node.Name, &createdAt, &node.PublicKey, &enrolledAt, &expiresAt,
		&node.Hostname, &node.OS, &node.Arch, &node.BuildVersion, &node.ProtocolMinor, &lastSeenAt, &sessionID); err != nil {
		return Node{}, err
	}
	node.SessionID = sessionID.String
	var err error
	if node.CreatedAt, err = parseTime(createdAt); err != nil {
		return Node{}, err
	}
	for _, field := range []struct {
		raw    sql.NullString
		target **time.Time
	}{{enrolledAt, &node.EnrolledAt}, {expiresAt, &node.EnrollmentExpiresAt}, {lastSeenAt, &node.LastSeenAt}} {
		if !field.raw.Valid {
			continue
		}
		value, err := parseTime(field.raw.String)
		if err != nil {
			return Node{}, err
		}
		*field.target = &value
	}
	return node, nil
}
