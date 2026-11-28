// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"slices"
	"strings"
	"time"
)

var (
	ErrNodeNotFound      = errors.New("node not found")
	ErrNodeNameConflict  = errors.New("a node with this name already exists")
	ErrEnrollmentInvalid = errors.New("enrollment token is invalid, used or expired")
)

// Node is one enrolled node: its identity key and what it last reported
// about itself.
type Node struct {
	ID            string
	Name          string
	CreatedAt     time.Time
	PublicKey     []byte
	EnrolledAt    time.Time
	Hostname      string
	OS            string
	Arch          string
	BuildVersion  string
	ProtocolMinor int
	LastSeenAt    *time.Time
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
	// Tags are sorted and distinct.
	Tags []string
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

// NewNodeEnrollment is a one-time token that creates node NodeID named Name
// when a host enrolls with it.
type NewNodeEnrollment struct {
	TokenHash [32]byte
	NodeID    string
	Name      string
	CreatedAt time.Time
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
	// Configuration, when set, is the first configuration file of a node the
	// token creates, saved in the same transaction as the node. A token that
	// enrolls an existing node ignores it.
	Configuration *ConfigurationFileUpdate
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

const nodeColumns = `id, name, created_at, public_key, enrolled_at,
	hostname, os, arch, build_version, protocol_minor, last_seen_at, session_id,
	instance_id, state_report, reported_at,
	traffic_quota_gib, traffic_period_months, traffic_reset_day, public_host, tags, settings_revision,
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

// CurrentPanelID returns the panel's identity without creating one; empty
// before any node was enrolled.
func (s *Store) CurrentPanelID(ctx context.Context) (string, error) {
	var id string
	err := s.db.QueryRowContext(ctx, `SELECT coalesce((SELECT panel_id FROM fleet_identity WHERE singleton = 1), '')`).Scan(&id)
	return id, err
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

// Limits of node tags.
const (
	MaximumNodeTags      = 16
	MaximumNodeTagLength = 32
)

// ValidNodeTag reports whether tag is 1 to 32 lowercase letters, digits,
// hyphens and underscores.
func ValidNodeTag(tag string) bool {
	if tag == "" || len(tag) > MaximumNodeTagLength {
		return false
	}
	for _, character := range tag {
		if (character < 'a' || character > 'z') && (character < '0' || character > '9') && character != '-' && character != '_' {
			return false
		}
	}
	return true
}

// NormalizeNodeTags returns tags sorted and without duplicates, or an error
// when one is invalid or there are more than 16.
func NormalizeNodeTags(tags []string) ([]string, error) {
	normalized := slices.Compact(append([]string{}, slices.Sorted(slices.Values(tags))...))
	if len(normalized) > MaximumNodeTags {
		return nil, fmt.Errorf("a node has at most %d tags", MaximumNodeTags)
	}
	for _, tag := range normalized {
		if !ValidNodeTag(tag) {
			return nil, errors.New("each tag must be 1 to 32 lowercase letters, digits, hyphens or underscores")
		}
	}
	return normalized, nil
}

// SaveNodeSettings replaces the node's settings when their revision is still
// expectedRevision. A zero quota is stored as unlimited; tags are stored
// sorted and distinct.
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
	tags, err := NormalizeNodeTags(value.Tags)
	if err != nil {
		return Node{}, fmt.Errorf("%w: %v", ErrNodeSettingsInvalid, err)
	}
	encodedTags, err := json.Marshal(tags)
	if err != nil {
		return Node{}, err
	}
	var quota any
	if traffic.QuotaGiB != nil {
		quota = *traffic.QuotaGiB
	}
	node, err := scanNodeResult(s.db.QueryRowContext(ctx, `UPDATE nodes SET traffic_quota_gib = ?, traffic_period_months = ?,
		traffic_reset_day = ?, public_host = ?, tags = ?, settings_revision = settings_revision + 1
		WHERE id = ? AND settings_revision = ? RETURNING `+nodeColumns,
		quota, traffic.PeriodMonths, traffic.ResetDay, host, string(encodedTags), id, expectedRevision))
	if errors.Is(err, ErrNodeNotFound) {
		if _, lookupErr := s.Node(ctx, id); lookupErr == nil {
			return Node{}, ErrNodeSettingsConflict
		}
	}
	return node, err
}

// DeleteNode removes the node with its unused enrollment token.
func (s *Store) DeleteNode(ctx context.Context, id string) error {
	return s.WithTx(ctx, func(tx *sql.Tx) error {
		if err := expectOneNode(tx.ExecContext(ctx, `DELETE FROM nodes WHERE id = ?`, id)); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `DELETE FROM node_enrollments WHERE node_id = ?`, id)
		return err
	})
}

// IssueNewNodeEnrollment records a token that creates a node when a host
// enrolls with it. The token replaces an unused one issued for the same name;
// it fails with ErrNodeNameConflict while a node has the name.
func (s *Store) IssueNewNodeEnrollment(ctx context.Context, input NewNodeEnrollment) error {
	return s.WithTx(ctx, func(tx *sql.Tx) error {
		if err := deleteExpiredNodeEnrollments(ctx, tx, input.CreatedAt); err != nil {
			return err
		}
		var used bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM nodes WHERE name = ?)`, input.Name).Scan(&used); err != nil {
			return err
		} else if used {
			return ErrNodeNameConflict
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM node_enrollments WHERE name = ?`, input.Name); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO node_enrollments(token_hash, node_id, name, created_at, expires_at) VALUES(?,?,?,?,?)`,
			input.TokenHash[:], input.NodeID, input.Name, formatTime(input.CreatedAt), formatTime(input.ExpiresAt))
		return err
	})
}

// IssueNodeEnrollment replaces the unused token of the enrolled node id. The
// enrolled key stays valid until a host enrolls with the new token.
func (s *Store) IssueNodeEnrollment(ctx context.Context, id string, tokenHash [32]byte, createdAt, expiresAt time.Time) (Node, error) {
	var node Node
	err := s.WithTx(ctx, func(tx *sql.Tx) error {
		var err error
		if node, err = scanNodeResult(tx.QueryRowContext(ctx, `SELECT `+nodeColumns+` FROM nodes WHERE id = ?`, id)); err != nil {
			return err
		}
		if err := deleteExpiredNodeEnrollments(ctx, tx, createdAt); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM node_enrollments WHERE node_id = ?`, id); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO node_enrollments(token_hash, node_id, created_at, expires_at) VALUES(?,?,?,?)`,
			tokenHash[:], id, formatTime(createdAt), formatTime(expiresAt))
		return err
	})
	return node, err
}

// CancelNodeEnrollment discards the unused token issued for node id, if
// any, and reports whether there was one: a new node's token then creates
// nothing, and an enrolled node keeps its key.
func (s *Store) CancelNodeEnrollment(ctx context.Context, id string) (bool, error) {
	result, err := s.db.ExecContext(ctx, `DELETE FROM node_enrollments WHERE node_id = ?`, id)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	return rows > 0, err
}

// EnrollNode consumes an unexpired enrollment token. A new node's token
// creates the node with the key, failing with ErrNodeNameConflict and keeping
// the token when another node has taken its name; an enrolled node's token
// replaces its key. created reports which one happened.
func (s *Store) EnrollNode(ctx context.Context, tokenHash [32]byte, input NodeEnrollment) (node Node, created bool, err error) {
	err = s.WithTx(ctx, func(tx *sql.Tx) error {
		if err := deleteExpiredNodeEnrollments(ctx, tx, input.EnrolledAt); err != nil {
			return err
		}
		var id string
		var name sql.NullString
		err := tx.QueryRowContext(ctx, `DELETE FROM node_enrollments WHERE token_hash = ? RETURNING node_id, name`, tokenHash[:]).Scan(&id, &name)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrEnrollmentInvalid
		} else if err != nil {
			return err
		}
		enrolledAt := formatTime(input.EnrolledAt)
		if !name.Valid {
			node, err = scanNodeResult(tx.QueryRowContext(ctx, `UPDATE nodes SET public_key = ?, enrolled_at = ?, session_id = NULL,
				hostname = ?, os = ?, arch = ?, build_version = ?, protocol_minor = ?
				WHERE id = ? RETURNING `+nodeColumns,
				input.PublicKey, enrolledAt, input.Hostname, input.OS, input.Arch, input.BuildVersion, input.ProtocolMinor, id))
			if errors.Is(err, ErrNodeNotFound) {
				return ErrEnrollmentInvalid
			}
			return err
		}
		created = true
		node, err = scanNodeResult(tx.QueryRowContext(ctx, `INSERT INTO nodes(id, name, created_at, public_key, enrolled_at,
			hostname, os, arch, build_version, protocol_minor) VALUES(?,?,?,?,?,?,?,?,?,?) RETURNING `+nodeColumns,
			id, name.String, enrolledAt, input.PublicKey, enrolledAt,
			input.Hostname, input.OS, input.Arch, input.BuildVersion, input.ProtocolMinor))
		if err != nil || input.Configuration == nil {
			return err
		}
		initial := input.Configuration
		_, err = saveConfigurationFileTx(ctx, tx, node.ID, initial.ExpectedRevision, initial.Content, initial.Revision)
		return err
	})
	if err != nil {
		return Node{}, false, err
	}
	return node, created, nil
}

// deleteExpiredNodeEnrollments removes the tokens that expired by now.
func deleteExpiredNodeEnrollments(ctx context.Context, tx *sql.Tx, now time.Time) error {
	_, err := tx.ExecContext(ctx, `DELETE FROM node_enrollments WHERE expires_at <= ?`, formatTime(now))
	return err
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
// session sessionID, while that is still its session. published reports
// whether what its subscription inbounds derive from changed: the applied
// bundle or the reported public IP.
func (s *Store) RecordNodeStateReport(ctx context.Context, id, sessionID string, report []byte, summary NodeReport, reportedAt time.Time) (published bool, err error) {
	applied := summary.Applied
	err = s.WithTx(ctx, func(tx *sql.Tx) error {
		var bundle, revision, digest, core sql.NullString
		var publicIP string
		err := tx.QueryRowContext(ctx, `SELECT applied_bundle_id, applied_revision_id, applied_config_sha256, applied_core_version, reported_public_ip
			FROM nodes WHERE id = ? AND session_id = ?`, id, sessionID).Scan(&bundle, &revision, &digest, &core, &publicIP)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNodeNotFound
		} else if err != nil {
			return err
		}
		published = bundle.String != applied.ID || revision.String != applied.RevisionID || digest.String != applied.ConfigSHA256 ||
			core.String != applied.ExactCoreVersion || publicIP != summary.PublicIP
		return expectOneNode(tx.ExecContext(ctx, `UPDATE nodes SET state_report = ?, reported_at = ?, last_seen_at = ?,
			applied_bundle_id = ?, applied_revision_id = ?, applied_config_sha256 = ?, applied_core_version = ?, reported_public_ip = ?
			WHERE id = ? AND session_id = ?`, report, formatTime(reportedAt), formatTime(reportedAt),
			nullIfEmpty(applied.ID), nullIfEmpty(applied.RevisionID), nullIfEmpty(applied.ConfigSHA256), nullIfEmpty(applied.ExactCoreVersion),
			summary.PublicIP, id, sessionID))
	})
	return published && err == nil, err
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
	var createdAt, enrolledAt string
	var lastSeenAt, sessionID, instanceID, reportedAt sql.NullString
	var appliedID, appliedRevision, appliedSHA256, appliedVersion sql.NullString
	var quota sql.NullInt64
	var tags string
	settings := &node.Settings
	if err := row.Scan(&node.ID, &node.Name, &createdAt, &node.PublicKey, &enrolledAt,
		&node.Hostname, &node.OS, &node.Arch, &node.BuildVersion, &node.ProtocolMinor, &lastSeenAt, &sessionID,
		&instanceID, &node.StateReport, &reportedAt,
		&quota, &settings.Traffic.PeriodMonths, &settings.Traffic.ResetDay, &settings.PublicHost, &tags, &settings.Revision,
		&appliedID, &appliedRevision, &appliedSHA256, &appliedVersion, &node.Report.PublicIP); err != nil {
		return Node{}, err
	}
	if quota.Valid {
		settings.Traffic.QuotaGiB = &quota.Int64
	}
	if err := json.Unmarshal([]byte(tags), &settings.Tags); err != nil {
		return Node{}, fmt.Errorf("decode node tags: %w", err)
	}
	if settings.Tags == nil {
		settings.Tags = []string{}
	}
	node.Report.Applied = AppliedBundle{ID: appliedID.String, RevisionID: appliedRevision.String,
		ConfigSHA256: appliedSHA256.String, ExactCoreVersion: appliedVersion.String}
	node.SessionID, node.InstanceID = sessionID.String, instanceID.String
	var err error
	if node.CreatedAt, err = parseTime(createdAt); err != nil {
		return Node{}, err
	}
	if node.EnrolledAt, err = parseTime(enrolledAt); err != nil {
		return Node{}, err
	}
	for _, field := range []struct {
		raw    sql.NullString
		target **time.Time
	}{{lastSeenAt, &node.LastSeenAt}, {reportedAt, &node.ReportedAt}} {
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
