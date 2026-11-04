// SPDX-License-Identifier: GPL-3.0-or-later

// Package fleet owns the panel's nodes: the registry, enrollment, the node
// listener, its authenticated sessions and the commands sent over them.
package fleet

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/rehuony/sing-box-panel/internal/buildinfo"
	"github.com/rehuony/sing-box-panel/internal/configuration"
	"github.com/rehuony/sing-box-panel/internal/database"
	"github.com/rehuony/sing-box-panel/internal/nodeapi"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/corecache"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/nodeconfig"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/settings"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/telemetry"
)

// EnrollmentLifetime is how long an enrollment code stays usable.
const EnrollmentLifetime = time.Hour

// MaximumNameLength bounds node names in characters.
const MaximumNameLength = 64

// Node states shown by the management API.
const (
	StatePending = "pending"
	StateOnline  = "online"
	StateOffline = "offline"
)

var (
	ErrNameInvalid      = errors.New("node name must be 1 to 64 characters without control characters or surrounding spaces")
	ErrEndpointRequired = errors.New("set server.external_origin or nodes.endpoint to an address that nodes can reach when server.host is an unspecified address")
)

// Node is one node as the management API shows it.
type Node struct {
	ID           string     `json:"id"`
	Name         string     `json:"name"`
	State        string     `json:"state"`
	CreatedAt    time.Time  `json:"created_at"`
	EnrolledAt   *time.Time `json:"enrolled_at"`
	LastSeenAt   *time.Time `json:"last_seen_at"`
	Hostname     string     `json:"hostname"`
	OS           string     `json:"os"`
	Arch         string     `json:"arch"`
	BuildVersion string     `json:"build_version"`
}

// Enrollment is a one-time code that enrolls, or re-enrolls, a node.
type Enrollment struct {
	Node      Node      `json:"node"`
	Code      string    `json:"code"`
	Command   string    `json:"command"`
	ExpiresAt time.Time `json:"expires_at"`
}

// Options supplies a Service's collaborators.
type Options struct {
	// Build is the panel's build, reported to nodes in Welcome.
	Build buildinfo.Info
	// Now and Random default to the system clock and crypto/rand.
	Now    func() time.Time
	Random func([]byte) (int, error)
}

// Service manages the panel's nodes.
type Service struct {
	database *store.Store
	settings *settings.Service
	records  *telemetry.Service
	configs  *nodeconfig.Service
	cores    *corecache.Service
	build    buildinfo.Info
	now      func() time.Time
	random   func([]byte) (int, error)
	sessions sessions
	grants   grants

	identityMu sync.Mutex
	identity   *Identity
	panelID    string
}

// New manages the nodes recorded in database. The node TLS identity lives
// in the data directory of panelSettings; configs holds each node's
// configuration and cores the archives nodes install from.
func New(database *store.Store, panelSettings *settings.Service, records *telemetry.Service,
	configs *nodeconfig.Service, cores *corecache.Service, options Options,
) *Service {
	service := &Service{
		database: database, settings: panelSettings, records: records, configs: configs, cores: cores, build: options.Build,
		now: time.Now, random: rand.Read, sessions: sessions{byNode: map[string]*session{}},
	}
	if options.Now != nil {
		service.now = options.Now
	}
	if options.Random != nil {
		service.random = options.Random
	}
	return service
}

// Identity returns the node TLS identity, creating it on first use.
func (service *Service) Identity() (Identity, error) {
	service.identityMu.Lock()
	defer service.identityMu.Unlock()
	if service.identity == nil {
		identity, err := LoadOrCreateIdentity(service.settings.Value().DataDir)
		if err != nil {
			return Identity{}, err
		}
		service.identity = &identity
	}
	return *service.identity, nil
}

// PanelID returns the identity that nodes bind their session proofs to.
func (service *Service) PanelID(ctx context.Context) (string, error) {
	service.identityMu.Lock()
	defer service.identityMu.Unlock()
	if service.panelID == "" {
		candidate, err := database.NewID(service.random, "panel")
		if err != nil {
			return "", err
		}
		id, err := service.database.PanelID(ctx, candidate)
		if err != nil {
			return "", err
		}
		service.panelID = id
	}
	return service.panelID, nil
}

func (service *Service) Nodes(ctx context.Context) ([]Node, error) {
	records, err := service.database.Nodes(ctx)
	if err != nil {
		return nil, err
	}
	nodes := make([]Node, 0, len(records))
	for _, record := range records {
		nodes = append(nodes, service.view(record))
	}
	return nodes, nil
}

func (service *Service) Node(ctx context.Context, id string) (Node, error) {
	record, err := service.database.Node(ctx, id)
	if err != nil {
		return Node{}, err
	}
	return service.view(record), nil
}

// FindNode looks a node up by its ID or, failing that, its name.
func (service *Service) FindNode(ctx context.Context, idOrName string) (Node, error) {
	record, err := service.database.Node(ctx, idOrName)
	if errors.Is(err, store.ErrNodeNotFound) {
		record, err = service.database.NodeByName(ctx, idOrName)
	}
	if err != nil {
		return Node{}, err
	}
	return service.view(record), nil
}

// AddNode registers a node and returns the code that enrolls it.
func (service *Service) AddNode(ctx context.Context, name string) (result Enrollment, err error) {
	defer func() {
		service.records.RecordOperation(ctx, "node.create", "Node creation", err, telemetry.OperationLogContext{NodeID: result.Node.ID})
	}()
	if !validName(name) {
		return Enrollment{}, ErrNameInvalid
	}
	code, err := service.newCode(ctx)
	if err != nil {
		return Enrollment{}, err
	}
	id, err := database.NewID(service.random, "node")
	if err != nil {
		return Enrollment{}, err
	}
	now := service.now().UTC()
	// The node starts with the empty document, saved with it.
	initial, err := store.NewConfigurationFileUpdate(service.random, now, 0, string(configuration.Empty().CanonicalJSON()))
	if err != nil {
		return Enrollment{}, err
	}
	record, err := service.database.CreateNode(ctx, store.NewNode{
		ID: id, Name: name, CreatedAt: now, TokenHash: nodeapi.TokenHash(code.Token), ExpiresAt: now.Add(EnrollmentLifetime),
		Configuration: initial,
	})
	if err != nil {
		return Enrollment{}, err
	}
	return service.enrollment(record, code), nil
}

// IssueEnrollment returns a new code for an existing node. Enrolling with it
// replaces the node's key and ends its session.
func (service *Service) IssueEnrollment(ctx context.Context, id string) (result Enrollment, err error) {
	defer func() {
		service.records.RecordOperation(ctx, "node.enrollment.issue", "Node enrollment code", err, telemetry.OperationLogContext{NodeID: id})
	}()
	code, err := service.newCode(ctx)
	if err != nil {
		return Enrollment{}, err
	}
	record, err := service.database.IssueNodeEnrollment(ctx, id, nodeapi.TokenHash(code.Token), service.now().UTC().Add(EnrollmentLifetime))
	if err != nil {
		return Enrollment{}, err
	}
	return service.enrollment(record, code), nil
}

func (service *Service) RenameNode(ctx context.Context, id, name string) (result Node, err error) {
	defer func() {
		service.records.RecordOperation(ctx, "node.rename", "Node rename", err, telemetry.OperationLogContext{NodeID: id})
	}()
	if !validName(name) {
		return Node{}, ErrNameInvalid
	}
	record, err := service.database.RenameNode(ctx, id, name)
	if err != nil {
		return Node{}, err
	}
	return service.view(record), nil
}

// DeleteNode removes a node and its key and ends its session.
func (service *Service) DeleteNode(ctx context.Context, id string) (err error) {
	defer func() {
		service.records.RecordOperation(ctx, "node.delete", "Node deletion", err, telemetry.OperationLogContext{NodeID: id})
	}()
	if err := service.database.DeleteNode(ctx, id); err != nil {
		return err
	}
	service.sessions.close(id, errNodeRemoved)
	return nil
}

// newCode builds an enrollment code with a fresh token for the current
// server and node settings.
func (service *Service) newCode(ctx context.Context) (nodeapi.EnrollmentCode, error) {
	value, err := service.settings.EffectiveSettings(ctx)
	if err != nil {
		return nodeapi.EnrollmentCode{}, err
	}
	endpoint := value.NodeEndpoint()
	host, _, err := net.SplitHostPort(endpoint)
	if err != nil {
		return nodeapi.EnrollmentCode{}, err
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsUnspecified() {
		return nodeapi.EnrollmentCode{}, ErrEndpointRequired
	}
	raw := make([]byte, nodeapi.TokenSize)
	if n, err := service.random(raw); err != nil || n != len(raw) {
		return nodeapi.EnrollmentCode{}, errors.Join(errors.New("generate enrollment token"), err)
	}
	code := nodeapi.EnrollmentCode{Endpoint: endpoint, Path: value.Server.BasePath, Token: base64.RawURLEncoding.EncodeToString(raw)}
	if value.NodeTrust() == settings.NodeTrustPin {
		identity, err := service.Identity()
		if err != nil {
			return nodeapi.EnrollmentCode{}, err
		}
		code.Pin = identity.Pin
	}
	return code, nil
}

func (service *Service) enrollment(record store.Node, code nodeapi.EnrollmentCode) Enrollment {
	encoded := code.String()
	return Enrollment{
		Node: service.view(record), Code: encoded,
		Command: "sing-box-node enroll --code " + encoded, ExpiresAt: *record.EnrollmentExpiresAt,
	}
}

func (service *Service) view(record store.Node) Node {
	state := StateOffline
	switch {
	case record.EnrolledAt == nil:
		state = StatePending
	case record.SessionID != "":
		state = StateOnline
	}
	return Node{
		ID: record.ID, Name: record.Name, State: state, CreatedAt: record.CreatedAt,
		EnrolledAt: record.EnrolledAt, LastSeenAt: record.LastSeenAt,
		Hostname: record.Hostname, OS: record.OS, Arch: record.Arch, BuildVersion: record.BuildVersion,
	}
}

func validName(name string) bool {
	if name == "" || strings.TrimSpace(name) != name || !utf8.ValidString(name) || utf8.RuneCountInString(name) > MaximumNameLength {
		return false
	}
	for _, character := range name {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}
