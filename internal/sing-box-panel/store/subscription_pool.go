// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

var (
	ErrSubscriptionNodeNotFound = errors.New("subscription node not found")
	// ErrSubscriptionNodeNameTaken rejects a display name another pool node
	// uses, ignoring case.
	ErrSubscriptionNodeNameTaken = errors.New("subscription node display name is taken")
	// ErrSubscriptionPoolChanged rejects a pool order that does not name
	// exactly the pool's current nodes.
	ErrSubscriptionPoolChanged = errors.New("subscription node pool changed")
	// ErrSubscriptionPoolInputsChanged means the inputs a synchronization was
	// computed from changed before it could be saved.
	ErrSubscriptionPoolInputsChanged = errors.New("subscription node pool inputs changed")
)

// MaximumNodeDisplayNameBytes bounds a pool node's display name.
const MaximumNodeDisplayNameBytes = 256

const (
	maximumOriginKeyBytes  = 1024
	maximumOriginNameBytes = 512
)

// SubscriptionPoolNode is one node of the subscription node pool.
type SubscriptionPoolNode struct {
	ID string
	// NodeID and NodeName name the fleet node of an inbound; SourceID and
	// SourceName the source of a source node.
	NodeID      string
	NodeName    string
	SourceID    string
	SourceName  string
	OriginKey   string
	OriginName  string
	DisplayName string
	Protocol    string
	// Outbound is the sing-box outbound published for the node, without its tag.
	Outbound  json.RawMessage
	Position  int64
	CreatedAt time.Time
	UpdatedAt time.Time
}

// SubscriptionPoolInputs is everything the pool is synchronized from, read
// at one point in time, and the fingerprint that identifies that state.
type SubscriptionPoolInputs struct {
	Fingerprint  string
	Publications []NodePublication
	Sources      []SubscriptionPoolSource
}

// SubscriptionPoolSource is a source's stored content as synchronization
// reads it.
type SubscriptionPoolSource struct {
	ID            string
	Name          string
	ContentFormat string
	Filter        SubscriptionSourceFilter
	Content       []byte
}

// SubscriptionPoolNodeInput is a node the pool's inputs yield.
type SubscriptionPoolNodeInput struct {
	ID         string
	NodeID     string
	SourceID   string
	OriginKey  string
	OriginName string
	// DisplayName is the name a new node takes, before ` 2`, ` 3` and so on
	// are appended while another node has it.
	DisplayName string
	Protocol    string
	Outbound    json.RawMessage
}

// SubscriptionPoolSynchronization is the pool its inputs yield, computed from
// the inputs with Fingerprint.
type SubscriptionPoolSynchronization struct {
	Fingerprint string
	// Nodes are every node the inputs yield, in origin order.
	Nodes []SubscriptionPoolNodeInput
	// KeptSources are sources whose content no longer reads: their nodes stay
	// as they are, and SourceErrors records why.
	KeptSources  map[string]bool
	SourceErrors map[string]string
	// KeptNodes are fleet nodes whose public host is not known right now,
	// with neither an override nor a reported public IP: their nodes stay as
	// they are until it is.
	KeptNodes map[string]bool
	At        time.Time
}

const subscriptionPoolColumns = `
    p.id, coalesce(p.node_id, ''), coalesce(n.name, ''), coalesce(p.source_id, ''), coalesce(s.name, ''),
    p.origin_key, p.origin_name, p.display_name, p.protocol, p.outbound_json, p.position, p.created_at, p.updated_at`

const subscriptionPoolFrom = ` FROM subscription_nodes AS p
    LEFT JOIN nodes AS n ON n.id = p.node_id
    LEFT JOIN subscription_sources AS s ON s.id = p.source_id`

// SubscriptionPool returns the pool in its saved order.
func (s *Store) SubscriptionPool(ctx context.Context) ([]SubscriptionPoolNode, error) {
	return loadSubscriptionPool(ctx, s.db)
}

func (s *Store) SubscriptionPoolNode(ctx context.Context, id string) (SubscriptionPoolNode, error) {
	return getSubscriptionPoolNode(ctx, s.db, id)
}

type queryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func loadSubscriptionPool(ctx context.Context, q queryer) ([]SubscriptionPoolNode, error) {
	rows, err := q.QueryContext(ctx, `SELECT `+subscriptionPoolColumns+subscriptionPoolFrom+` ORDER BY p.position, p.id`)
	if err != nil {
		return nil, fmt.Errorf("read subscription node pool: %w", err)
	}
	defer rows.Close()
	nodes := []SubscriptionPoolNode{}
	for rows.Next() {
		node, err := scanSubscriptionPoolNode(rows)
		if err != nil {
			return nil, fmt.Errorf("scan subscription node: %w", err)
		}
		nodes = append(nodes, node)
	}
	return nodes, rows.Err()
}

func getSubscriptionPoolNode(ctx context.Context, q queryRower, id string) (SubscriptionPoolNode, error) {
	node, err := scanSubscriptionPoolNode(q.QueryRowContext(ctx, `SELECT `+subscriptionPoolColumns+subscriptionPoolFrom+` WHERE p.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return SubscriptionPoolNode{}, fmt.Errorf("%w: %s", ErrSubscriptionNodeNotFound, id)
	}
	if err != nil {
		return SubscriptionPoolNode{}, fmt.Errorf("get subscription node: %w", err)
	}
	return node, nil
}

func scanSubscriptionPoolNode(row rowScanner) (SubscriptionPoolNode, error) {
	var node SubscriptionPoolNode
	var outbound, createdAt, updatedAt string
	if err := row.Scan(&node.ID, &node.NodeID, &node.NodeName, &node.SourceID, &node.SourceName,
		&node.OriginKey, &node.OriginName, &node.DisplayName, &node.Protocol, &outbound, &node.Position,
		&createdAt, &updatedAt); err != nil {
		return SubscriptionPoolNode{}, err
	}
	node.Outbound = json.RawMessage(outbound)
	var err error
	if node.CreatedAt, err = parseTime(createdAt); err != nil {
		return SubscriptionPoolNode{}, err
	}
	if node.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return SubscriptionPoolNode{}, err
	}
	return node, nil
}

// RenameSubscriptionPoolNode changes a node's display name while its
// updated_at is still expected.
func (s *Store) RenameSubscriptionPoolNode(ctx context.Context, id, displayName string, expected, updatedAt time.Time) (SubscriptionPoolNode, error) {
	if err := ValidateNodeDisplayName(displayName); err != nil {
		return SubscriptionPoolNode{}, err
	}
	expected, err := requiredUTC(expected, "expected node updated_at")
	if err != nil {
		return SubscriptionPoolNode{}, err
	}
	updated, err := nextSubscriptionUpdateTime(updatedAt, expected)
	if err != nil {
		return SubscriptionPoolNode{}, err
	}
	var stored SubscriptionPoolNode
	err = s.WithTx(ctx, func(tx *sql.Tx) error {
		current, err := getSubscriptionPoolNode(ctx, tx, id)
		if err != nil {
			return err
		}
		if !current.UpdatedAt.Equal(expected) {
			return subscriptionConflict("node", id, expected, current.UpdatedAt)
		}
		var taken bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM subscription_nodes WHERE display_name = ? AND id <> ?)`,
			displayName, id).Scan(&taken); err != nil {
			return err
		}
		if taken {
			return ErrSubscriptionNodeNameTaken
		}
		if _, err := tx.ExecContext(ctx, `UPDATE subscription_nodes SET display_name = ?, updated_at = ? WHERE id = ?`,
			displayName, formatTime(updated), id); err != nil {
			return fmt.Errorf("rename subscription node: %w", err)
		}
		stored, err = getSubscriptionPoolNode(ctx, tx, id)
		return err
	})
	return stored, err
}

// ValidateNodeDisplayName accepts 1 to 256 bytes of UTF-8 without
// surrounding spaces or control characters.
func ValidateNodeDisplayName(value string) error {
	if value == "" || len(value) > MaximumNodeDisplayNameBytes || strings.TrimSpace(value) != value || !utf8.ValidString(value) ||
		strings.IndexFunc(value, unicode.IsControl) >= 0 {
		return errors.New("a subscription node display name must be 1 to 256 bytes without surrounding spaces or control characters")
	}
	return nil
}

// SetSubscriptionPoolOrder saves the pool's order. ids must name every node
// of the pool exactly once.
func (s *Store) SetSubscriptionPoolOrder(ctx context.Context, ids []string) error {
	return s.WithTx(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `SELECT id FROM subscription_nodes`)
		if err != nil {
			return err
		}
		current := map[string]bool{}
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				_ = rows.Close()
				return err
			}
			current[id] = true
		}
		if err := rows.Close(); err != nil {
			return err
		}
		if err := rows.Err(); err != nil {
			return err
		}
		if len(ids) != len(current) {
			return ErrSubscriptionPoolChanged
		}
		for _, id := range ids {
			if !current[id] {
				return ErrSubscriptionPoolChanged
			}
			delete(current, id)
		}
		statement, err := tx.PrepareContext(ctx, `UPDATE subscription_nodes SET position = ? WHERE id = ?`)
		if err != nil {
			return err
		}
		defer statement.Close()
		for position, id := range ids {
			if _, err := statement.ExecContext(ctx, position, id); err != nil {
				return fmt.Errorf("save subscription node order: %w", err)
			}
		}
		return nil
	})
}

// LoadSubscriptionPoolInputs reads the pool's inputs in one transaction:
// every fleet node that reports an applied configuration, every source with
// its stored content, and their fingerprint.
func (s *Store) LoadSubscriptionPoolInputs(ctx context.Context) (SubscriptionPoolInputs, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return SubscriptionPoolInputs{}, fmt.Errorf("begin subscription pool read: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var inputs SubscriptionPoolInputs
	if inputs.Fingerprint, err = subscriptionPoolFingerprint(ctx, tx); err != nil {
		return SubscriptionPoolInputs{}, err
	}
	if inputs.Publications, err = loadNodePublications(ctx, tx); err != nil {
		return SubscriptionPoolInputs{}, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT s.id, s.name, s.content_format, s.include_rules, s.exclude_rules, s.content
		FROM subscription_sources AS s ORDER BY s.created_at, s.id`)
	if err != nil {
		return SubscriptionPoolInputs{}, fmt.Errorf("read subscription sources: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var source SubscriptionPoolSource
		var include, exclude string
		if err := rows.Scan(&source.ID, &source.Name, &source.ContentFormat, &include, &exclude, &source.Content); err != nil {
			return SubscriptionPoolInputs{}, err
		}
		if err := json.Unmarshal([]byte(include), &source.Filter.Include); err != nil {
			return SubscriptionPoolInputs{}, err
		}
		if err := json.Unmarshal([]byte(exclude), &source.Filter.Exclude); err != nil {
			return SubscriptionPoolInputs{}, err
		}
		inputs.Sources = append(inputs.Sources, source)
	}
	if err := rows.Err(); err != nil {
		return SubscriptionPoolInputs{}, err
	}
	return inputs, tx.Commit()
}

// subscriptionPoolFingerprint digests what the pool derives from: each fleet
// node's name, hosts and applied configuration, and each source's content
// and filter.
func subscriptionPoolFingerprint(ctx context.Context, tx *sql.Tx) (string, error) {
	digest := sha256.New()
	for _, query := range []string{
		`SELECT id, name, public_host, reported_public_ip, coalesce(applied_bundle_id, ''), coalesce(applied_revision_id, ''),
		        coalesce(applied_config_sha256, ''), coalesce(applied_core_version, '') FROM nodes ORDER BY id`,
		`SELECT id, name, content_format, content_sha256, include_rules, exclude_rules, created_at, '', ''
		   FROM subscription_sources ORDER BY id`,
	} {
		rows, err := tx.QueryContext(ctx, query)
		if err != nil {
			return "", fmt.Errorf("read subscription pool inputs: %w", err)
		}
		columns, err := rows.Columns()
		if err != nil {
			_ = rows.Close()
			return "", err
		}
		values := make([]string, len(columns))
		targets := make([]any, len(columns))
		for index := range values {
			targets[index] = &values[index]
		}
		for rows.Next() {
			if err := rows.Scan(targets...); err != nil {
				_ = rows.Close()
				return "", err
			}
			for _, value := range values {
				digest.Write([]byte(strconv.Itoa(len(value))))
				digest.Write([]byte{':'})
				digest.Write([]byte(value))
			}
		}
		if err := rows.Close(); err != nil {
			return "", err
		}
		if err := rows.Err(); err != nil {
			return "", err
		}
		digest.Write([]byte{0})
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

// SynchronizeSubscriptionPool makes the pool what its inputs yield, when the
// inputs are still those the synchronization was computed from: it deletes
// the nodes that are gone, which removes them from every channel and grant,
// keeps those of kept sources and fleet nodes, replaces the content of the
// nodes that remain and appends new nodes in origin order with free display
// names. The pool holds at most MaximumSubscriptionNodes.
func (s *Store) SynchronizeSubscriptionPool(ctx context.Context, input SubscriptionPoolSynchronization) error {
	at, err := requiredUTC(input.At, "subscription pool synchronization time")
	if err != nil {
		return err
	}
	for _, node := range input.Nodes {
		if err := validatePoolNodeInput(node); err != nil {
			return err
		}
	}
	return s.WithTx(ctx, func(tx *sql.Tx) error {
		fingerprint, err := subscriptionPoolFingerprint(ctx, tx)
		if err != nil {
			return err
		}
		if fingerprint != input.Fingerprint {
			return ErrSubscriptionPoolInputsChanged
		}
		type existingNode struct {
			nodeID, sourceID, displayName string
		}
		rows, err := tx.QueryContext(ctx, `SELECT id, coalesce(node_id, ''), coalesce(source_id, ''), display_name FROM subscription_nodes`)
		if err != nil {
			return err
		}
		existing := map[string]existingNode{}
		for rows.Next() {
			var id string
			var node existingNode
			if err := rows.Scan(&id, &node.nodeID, &node.sourceID, &node.displayName); err != nil {
				_ = rows.Close()
				return err
			}
			existing[id] = node
		}
		if err := rows.Close(); err != nil {
			return err
		}
		if err := rows.Err(); err != nil {
			return err
		}
		desired := make(map[string]bool, len(input.Nodes))
		for _, node := range input.Nodes {
			if desired[node.ID] {
				return fmt.Errorf("subscription pool synchronization repeats node %s", node.ID)
			}
			desired[node.ID] = true
		}
		taken := map[string]bool{}
		gone := []string{}
		kept := 0
		for id, node := range existing {
			switch {
			case desired[id]:
			case input.KeptSources[node.sourceID] || input.KeptNodes[node.nodeID]:
				kept++
			default:
				gone = append(gone, id)
				continue
			}
			taken[foldDisplayName(node.displayName)] = true
		}
		if len(input.Nodes)+kept > MaximumSubscriptionNodes {
			return fmt.Errorf("%w: the pool would hold more than %d nodes", ErrSubscriptionLimitExceeded, MaximumSubscriptionNodes)
		}
		slices.Sort(gone)
		for _, id := range gone {
			if _, err := tx.ExecContext(ctx, `DELETE FROM subscription_nodes WHERE id = ?`, id); err != nil {
				return fmt.Errorf("delete subscription node: %w", err)
			}
		}
		var position int64
		if err := tx.QueryRowContext(ctx, `SELECT coalesce(max(position) + 1, 0) FROM subscription_nodes`).Scan(&position); err != nil {
			return err
		}
		for _, node := range input.Nodes {
			if _, present := existing[node.ID]; present {
				if _, err := tx.ExecContext(ctx, `UPDATE subscription_nodes SET origin_name = ?, protocol = ?, outbound_json = ?
					WHERE id = ? AND (origin_name <> ? OR protocol <> ? OR outbound_json <> ?)`,
					node.OriginName, node.Protocol, string(node.Outbound), node.ID,
					node.OriginName, node.Protocol, string(node.Outbound)); err != nil {
					return fmt.Errorf("update subscription node: %w", err)
				}
				continue
			}
			name := freeDisplayName(node.DisplayName, taken)
			taken[foldDisplayName(name)] = true
			if _, err := tx.ExecContext(ctx, `INSERT INTO subscription_nodes(
					id, node_id, source_id, origin_key, origin_name, display_name, protocol, outbound_json, position, created_at, updated_at
				) VALUES (?, NULLIF(?, ''), NULLIF(?, ''), ?, ?, ?, ?, ?, ?, ?, ?)`,
				node.ID, node.NodeID, node.SourceID, node.OriginKey, node.OriginName, name, node.Protocol,
				string(node.Outbound), position, formatTime(at), formatTime(at)); err != nil {
				return fmt.Errorf("insert subscription node: %w", err)
			}
			position++
		}
		for id, code := range input.SourceErrors {
			if err := validateSourceError(code); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `UPDATE subscription_sources SET last_error = ? WHERE id = ?`, code, id); err != nil {
				return fmt.Errorf("record subscription source error: %w", err)
			}
		}
		return nil
	})
}

func validatePoolNodeInput(node SubscriptionPoolNodeInput) error {
	if node.ID == "" || len(node.ID) > 128 || (node.NodeID == "") == (node.SourceID == "") ||
		node.OriginKey == "" || len(node.OriginKey) > maximumOriginKeyBytes ||
		node.OriginName == "" || len(node.OriginName) > maximumOriginNameBytes ||
		node.DisplayName == "" || node.Protocol == "" {
		return fmt.Errorf("invalid subscription pool node %q", node.ID)
	}
	if !json.Valid(node.Outbound) || len(node.Outbound) == 0 || node.Outbound[0] != '{' {
		return fmt.Errorf("subscription pool node %q outbound is not a JSON object", node.ID)
	}
	return nil
}

// foldDisplayName folds ASCII letters, as SQLite's NOCASE collation does.
func foldDisplayName(value string) string {
	return strings.Map(func(character rune) rune {
		if character >= 'A' && character <= 'Z' {
			return character + ('a' - 'A')
		}
		return character
	}, value)
}

// freeDisplayName is base, or base followed by ` 2`, ` 3` and so on, the
// first name taken does not hold, within the display name limit.
func freeDisplayName(base string, taken map[string]bool) string {
	for number := 1; ; number++ {
		suffix := ""
		if number > 1 {
			suffix = " " + strconv.Itoa(number)
		}
		name := truncateUTF8(base, MaximumNodeDisplayNameBytes-len(suffix)) + suffix
		if !taken[foldDisplayName(name)] {
			return name
		}
	}
}

// truncateUTF8 shortens value to at most limit bytes without splitting a
// character or leaving trailing spaces.
func truncateUTF8(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	end := limit
	for end > 0 && !utf8.RuneStart(value[end]) {
		end--
	}
	return strings.TrimRight(value[:end], " ")
}
