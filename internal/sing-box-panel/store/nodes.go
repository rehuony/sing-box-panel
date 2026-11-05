// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
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
	// InstanceID identifies the node database of its latest session.
	InstanceID string
	Settings   NodeSettings
	// StateReport is the encoded state the node last reported, kept to show
	// while the node is offline; nil before its first report.
	StateReport []byte
	ReportedAt  *time.Time
	// Report is what the panel reads from the last state report.
	Report NodeReport
}

// NodeSettings are the panel's settings for one node.
type NodeSettings struct {
	// PublicHost is the host subscriptions publish for the node's inbounds;
	// empty uses the public IP the node reports.
	PublicHost string
	Traffic    NodeTrafficSettings
	// Revision increases with every saved change.
	Revision int64
}

// NodeReport is the part of a node's state report that other records join
// against.
type NodeReport struct {
	PublicIP string
	Applied  AppliedBundle
}

// AppliedBundle is the bundle a node reports running; empty before it applied
// one.
type AppliedBundle struct {
	ID               string
	RevisionID       string
	ConfigSHA256     string
	ExactCoreVersion string
}

// NewNode is a node record with a pending enrollment token.
type NewNode struct {
	ID        string
	Name      string
	CreatedAt time.Time
	TokenHash [32]byte
	ExpiresAt time.Time
	// Configuration, when set, is the node's first configuration file,
	// saved in the same transaction as the node.
	Configuration *ConfigurationFileUpdate
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
	InstanceID    string
	OS            string
	Arch          string
	BuildVersion  string
	ProtocolMinor int
	SeenAt        time.Time
}

const nodeColumns = `id, name, created_at, public_key, enrolled_at, enrollment_expires_at,
	hostname, os, arch, build_version, protocol_minor, last_seen_at, session_id,
	instance_id, state_report, reported_at,
	traffic_quota_gib, traffic_period_months, traffic_reset_day, public_host, settings_revision,
	applied_bundle_id, applied_revision_id, applied_config_sha256, applied_core_version, reported_public_ip`

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
	var node Node
	err := s.WithTx(ctx, func(tx *sql.Tx) error {
		var err error
		node, err = scanNodeResult(tx.QueryRowContext(ctx, `INSERT INTO nodes(id, name, created_at, enrollment_token_hash, enrollment_expires_at)
			VALUES(?,?,?,?,?) RETURNING `+nodeColumns,
			input.ID, input.Name, formatTime(input.CreatedAt), input.TokenHash[:], formatTime(input.ExpiresAt)))
		if err != nil || input.Configuration == nil {
			return err
		}
		initial := input.Configuration
		_, err = saveConfigurationFileTx(ctx, tx, node.ID, initial.ExpectedRevision, initial.Content, initial.Revision)
		return err
	})
	return node, err
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

// NodeTrafficSettings is a node's own traffic quota and period.
type NodeTrafficSettings struct {
	// QuotaGiB is the quota in GiB; nil is unlimited.
	QuotaGiB *int64
	// PeriodMonths is the length of each period.
	PeriodMonths int
	// ResetDay is the day of the month, 1 to 28, on which periods start at
	// 00:00 UTC.
	ResetDay int
}

// Limits of node traffic settings.
const (
	MaximumTrafficQuotaGiB     = (1<<63 - 1) / (1 << 30)
	MaximumTrafficPeriodMonths = 120
	MaximumTrafficResetDay     = 28
)

var (
	ErrNodeSettingsInvalid  = errors.New("node settings are invalid")
	ErrNodeSettingsConflict = errors.New("the node's settings changed since they were read")
)

// SaveNodeSettings replaces the node's settings when their revision is still
// expectedRevision. A zero quota is stored as unlimited.
func (s *Store) SaveNodeSettings(ctx context.Context, id string, expectedRevision int64, value NodeSettings) (Node, error) {
	traffic := value.Traffic
	if traffic.QuotaGiB != nil && *traffic.QuotaGiB == 0 {
		traffic.QuotaGiB = nil
	}
	if (traffic.QuotaGiB != nil && (*traffic.QuotaGiB < 1 || *traffic.QuotaGiB > MaximumTrafficQuotaGiB)) ||
		traffic.PeriodMonths < 1 || traffic.PeriodMonths > MaximumTrafficPeriodMonths || traffic.ResetDay < 1 || traffic.ResetDay > MaximumTrafficResetDay {
		return Node{}, fmt.Errorf("%w: the traffic quota must be unlimited or 1 to 8589934591 GiB, the period 1 to 120 months and the reset day 1 to 28", ErrNodeSettingsInvalid)
	}
	host, err := NormalizePublishedHost(value.PublicHost)
	if err != nil {
		return Node{}, fmt.Errorf("%w: %v", ErrNodeSettingsInvalid, err)
	}
	var quota any
	if traffic.QuotaGiB != nil {
		quota = *traffic.QuotaGiB
	}
	node, err := scanNodeResult(s.db.QueryRowContext(ctx, `UPDATE nodes SET traffic_quota_gib = ?, traffic_period_months = ?,
		traffic_reset_day = ?, public_host = ?, settings_revision = settings_revision + 1
		WHERE id = ? AND settings_revision = ? RETURNING `+nodeColumns,
		quota, traffic.PeriodMonths, traffic.ResetDay, host, id, expectedRevision))
	if errors.Is(err, ErrNodeNotFound) {
		if _, lookupErr := s.Node(ctx, id); lookupErr == nil {
			return Node{}, ErrNodeSettingsConflict
		}
	}
	return node, err
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
	return expectOneNode(s.db.ExecContext(ctx, `UPDATE nodes SET session_id = ?, instance_id = ?, os = ?, arch = ?, build_version = ?, protocol_minor = ?, last_seen_at = ?
		WHERE id = ? AND public_key = ?`, input.ID, input.InstanceID, input.OS, input.Arch, input.BuildVersion, input.ProtocolMinor,
		formatTime(input.SeenAt), id, publicKey))
}

// RecordNodeStateReport stores the encoded state the node reported in
// session sessionID, while that is still its session.
func (s *Store) RecordNodeStateReport(ctx context.Context, id, sessionID string, report []byte, summary NodeReport, reportedAt time.Time) error {
	applied := summary.Applied
	return expectOneNode(s.db.ExecContext(ctx, `UPDATE nodes SET state_report = ?, reported_at = ?, last_seen_at = ?,
		applied_bundle_id = ?, applied_revision_id = ?, applied_config_sha256 = ?, applied_core_version = ?, reported_public_ip = ?
		WHERE id = ? AND session_id = ?`, report, formatTime(reportedAt), formatTime(reportedAt),
		nullIfEmpty(applied.ID), nullIfEmpty(applied.RevisionID), nullIfEmpty(applied.ConfigSHA256), nullIfEmpty(applied.ExactCoreVersion),
		summary.PublicIP, id, sessionID))
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
	var enrolledAt, expiresAt, lastSeenAt, sessionID, instanceID, reportedAt sql.NullString
	var appliedID, appliedRevision, appliedSHA256, appliedVersion sql.NullString
	var quota sql.NullInt64
	settings := &node.Settings
	if err := row.Scan(&node.ID, &node.Name, &createdAt, &node.PublicKey, &enrolledAt, &expiresAt,
		&node.Hostname, &node.OS, &node.Arch, &node.BuildVersion, &node.ProtocolMinor, &lastSeenAt, &sessionID,
		&instanceID, &node.StateReport, &reportedAt,
		&quota, &settings.Traffic.PeriodMonths, &settings.Traffic.ResetDay, &settings.PublicHost, &settings.Revision,
		&appliedID, &appliedRevision, &appliedSHA256, &appliedVersion, &node.Report.PublicIP); err != nil {
		return Node{}, err
	}
	if quota.Valid {
		settings.Traffic.QuotaGiB = &quota.Int64
	}
	node.Report.Applied = AppliedBundle{ID: appliedID.String, RevisionID: appliedRevision.String,
		ConfigSHA256: appliedSHA256.String, ExactCoreVersion: appliedVersion.String}
	node.SessionID, node.InstanceID = sessionID.String, instanceID.String
	var err error
	if node.CreatedAt, err = parseTime(createdAt); err != nil {
		return Node{}, err
	}
	for _, field := range []struct {
		raw    sql.NullString
		target **time.Time
	}{{enrolledAt, &node.EnrolledAt}, {expiresAt, &node.EnrollmentExpiresAt}, {lastSeenAt, &node.LastSeenAt}, {reportedAt, &node.ReportedAt}} {
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

// NormalizePublishedHost validates a host that subscriptions publish as a
// node's endpoint: an IP address or a DNS name, without scheme or port. The
// empty host is kept.
func NormalizePublishedHost(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	if value != strings.TrimSpace(value) || len(value) > 253 ||
		strings.ContainsAny(value, "/@?#[]") {
		return "", errors.New("the public host must be an IP address or DNS name without scheme or port")
	}
	if address := net.ParseIP(value); address != nil {
		if address.IsLoopback() || address.IsUnspecified() {
			return "", errors.New("the public host cannot be a loopback or unspecified address")
		}
		return strings.ToLower(value), nil
	}
	if strings.EqualFold(value, "localhost") {
		return "", errors.New("the public host cannot be localhost")
	}
	if strings.Contains(value, ":") || strings.HasPrefix(value, ".") || strings.HasSuffix(value, ".") {
		return "", errors.New("the public host is invalid")
	}
	for _, label := range strings.Split(value, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return "", errors.New("the public host is invalid")
		}
		for _, character := range label {
			if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') ||
				(character >= '0' && character <= '9') || character == '-' {
				continue
			}
			return "", errors.New("the public host is invalid")
		}
	}
	return strings.ToLower(value), nil
}
