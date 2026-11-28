// SPDX-License-Identifier: GPL-3.0-or-later

// Package fleet owns the panel's nodes: the registry, enrollment, the node
// endpoints, their authenticated sessions and the commands sent over them.
package fleet

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/rehuony/sing-box-panel/internal/buildinfo"
	"github.com/rehuony/sing-box-panel/internal/database"
	"github.com/rehuony/sing-box-panel/internal/nodeapi"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/corecache"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/nodeconfig"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/releases"
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
	StateOnline  = "online"
	StateOffline = "offline"
)

var (
	ErrNameInvalid = errors.New("node name must be 1 to 64 characters without control characters or surrounding spaces")
	// ErrNodeURLRequired rejects an enrollment code no node elsewhere could
	// use: nodes reach the panel through HTTPS, which only an external
	// origin names.
	ErrNodeURLRequired = errors.New("set server.external_origin to the HTTPS origin of the reverse proxy that nodes reach the panel through")
)

// Node is one node as the management API shows it.
type Node struct {
	ID           string     `json:"id"`
	Name         string     `json:"name"`
	State        string     `json:"state"`
	CreatedAt    time.Time  `json:"created_at"`
	EnrolledAt   time.Time  `json:"enrolled_at"`
	LastSeenAt   *time.Time `json:"last_seen_at"`
	Hostname     string     `json:"hostname"`
	OS           string     `json:"os"`
	Arch         string     `json:"arch"`
	BuildVersion string     `json:"build_version"`
	// Tags are the node's tags from its settings, sorted.
	Tags []string `json:"tags"`
	// The state the node last reported; stale while the node is offline.
	ReportedAt *time.Time     `json:"reported_at"`
	PublicIP   string         `json:"public_ip"`
	Runtime    *RuntimeStatus `json:"runtime,omitempty"`
	// UpdateAvailable reports a node whose version differs from the node
	// release the panel distributes.
	UpdateAvailable bool `json:"update_available"`
	// LastUpdate is the node's last update attempt as it reported it.
	LastUpdate *UpdateAttempt `json:"last_update"`
}

// Enrollment is a one-time code that enrolls a new node, which exists once a
// host enrolls with it, or enrolls an existing node again.
type Enrollment struct {
	NodeID    string    `json:"node_id"`
	Name      string    `json:"name"`
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
	// Releases distributes the node release; without it nodes cannot be
	// updated from the panel.
	Releases *releases.Service
	// PublicationChanged runs after what a node's subscription inbounds
	// derive from changed: its applied configuration, reported public IP or
	// public host. It runs within the node's session, so it returns at once
	// and leaves the work it starts to run apart.
	PublicationChanged func(context.Context)
}

// Service manages the panel's nodes.
type Service struct {
	database *store.Store
	settings *settings.Service
	records  *telemetry.Service
	configs  *nodeconfig.Service
	cores    *corecache.Service
	releases *releases.Service
	build    buildinfo.Info
	now      func() time.Time
	random   func([]byte) (int, error)
	sessions sessions
	grants   grants
	// published is Options.PublicationChanged.
	published func(context.Context)

	viewersMu    sync.Mutex
	viewers      map[string]int
	metricsFeeds map[string]*telemetry.Feed[MetricsStreamSnapshot]
	// statusFeed shares one fleet snapshot among everyone streaming it;
	// statusItems holds each node's item of the latest read.
	statusFeed  *telemetry.Feed[FleetSnapshot]
	statusMu    sync.Mutex
	statusItems map[string]statusEncoding

	// exchanges counts the node exchanges in progress; stopped ends when
	// Shutdown begins, and exchangesMu orders the two.
	exchangesMu sync.Mutex
	exchanges   sync.WaitGroup
	stopped     context.Context
	stop        context.CancelFunc

	panelIDMu sync.Mutex
	panelID   string
}

// New manages the nodes recorded in database. configs holds each node's
// configuration and cores the archives nodes install from.
func New(database *store.Store, panelSettings *settings.Service, records *telemetry.Service,
	configs *nodeconfig.Service, cores *corecache.Service, options Options,
) *Service {
	service := &Service{
		database: database, settings: panelSettings, records: records, configs: configs, cores: cores, releases: options.Releases, build: options.Build,
		now: time.Now, random: rand.Read, sessions: sessions{byNode: map[string]*session{}},
		viewers: map[string]int{}, metricsFeeds: map[string]*telemetry.Feed[MetricsStreamSnapshot]{},
		published: options.PublicationChanged,
	}
	if options.Now != nil {
		service.now = options.Now
	}
	if options.Random != nil {
		service.random = options.Random
	}
	service.statusFeed = telemetry.NewFeed(StatusStreamInterval, service.collectStatus)
	service.stopped, service.stop = context.WithCancel(context.Background())
	return service
}

// PanelID returns the identity that nodes bind their session proofs to.
func (service *Service) PanelID(ctx context.Context) (string, error) {
	service.panelIDMu.Lock()
	defer service.panelIDMu.Unlock()
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

// AddNode returns the code that enrolls a new node named name. The node
// exists, with a preassigned ID, once a host enrolls with the code; until
// then no list, status or command includes it. A new code for the same name
// replaces an unused one.
func (service *Service) AddNode(ctx context.Context, name string) (result Enrollment, err error) {
	defer func() {
		service.records.RecordOperation(ctx, "node.enrollment.issue", "Node enrollment code", err, telemetry.OperationLogContext{NodeID: result.NodeID})
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
	input := store.NewNodeEnrollment{
		TokenHash: nodeapi.TokenHash(code.Token), NodeID: id, Name: name, CreatedAt: now, ExpiresAt: now.Add(EnrollmentLifetime),
	}
	if err := service.database.IssueNewNodeEnrollment(ctx, input); err != nil {
		return Enrollment{}, err
	}
	return enrollment(id, name, code, input.ExpiresAt), nil
}

// IssueEnrollment returns a new code for an existing node, replacing an
// unused one. Enrolling with it replaces the node's key and ends its session.
func (service *Service) IssueEnrollment(ctx context.Context, id string) (result Enrollment, err error) {
	defer func() {
		service.records.RecordOperation(ctx, "node.enrollment.issue", "Node enrollment code", err, telemetry.OperationLogContext{NodeID: id})
	}()
	code, err := service.newCode(ctx)
	if err != nil {
		return Enrollment{}, err
	}
	now := service.now().UTC()
	expiresAt := now.Add(EnrollmentLifetime)
	record, err := service.database.IssueNodeEnrollment(ctx, id, nodeapi.TokenHash(code.Token), now, expiresAt)
	if err != nil {
		return Enrollment{}, err
	}
	return enrollment(record.ID, record.Name, code, expiresAt), nil
}

// CancelEnrollment discards the unused code issued for node id, if any: a
// new node's code then creates nothing, and an enrolled node keeps its key.
// Only a discarded code or a failure is recorded.
func (service *Service) CancelEnrollment(ctx context.Context, id string) error {
	canceled, err := service.database.CancelNodeEnrollment(ctx, id)
	if canceled || err != nil {
		service.records.RecordOperation(ctx, "node.enrollment.cancel", "Node enrollment code cancellation", err, telemetry.OperationLogContext{NodeID: id})
	}
	return err
}

func (service *Service) RenameNode(ctx context.Context, id, name string) (result Node, err error) {
	defer func() {
		service.expireStatus()
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
		service.expireStatus()
		service.records.RecordOperation(ctx, "node.delete", "Node deletion", err, telemetry.OperationLogContext{NodeID: id})
	}()
	if err := service.database.DeleteNode(ctx, id); err != nil {
		return err
	}
	service.sessions.close(id, errNodeRemoved)
	service.records.ForgetNode(id)
	return nil
}

// newCode builds an enrollment code with a fresh token for the current
// server settings.
func (service *Service) newCode(ctx context.Context) (nodeapi.EnrollmentCode, error) {
	value, err := service.settings.EffectiveSettings(ctx)
	if err != nil {
		return nodeapi.EnrollmentCode{}, err
	}
	panelURL, ok := value.NodeURL()
	if !ok {
		return nodeapi.EnrollmentCode{}, ErrNodeURLRequired
	}
	raw := make([]byte, nodeapi.TokenSize)
	if n, err := service.random(raw); err != nil || n != len(raw) {
		return nodeapi.EnrollmentCode{}, errors.Join(errors.New("generate enrollment token"), err)
	}
	return nodeapi.EnrollmentCode{URL: panelURL, Token: base64.RawURLEncoding.EncodeToString(raw)}, nil
}

func enrollment(id, name string, code nodeapi.EnrollmentCode, expiresAt time.Time) Enrollment {
	encoded := code.String()
	return Enrollment{NodeID: id, Name: name, Code: encoded, Command: "sing-box-node enroll --code " + encoded, ExpiresAt: expiresAt}
}

func (service *Service) view(record store.Node) Node {
	return nodeView(record, nodeState(record), service.releaseVersion())
}

// nodeView shows record with reported, the state decoded from its last
// report, against release, the node release the panel distributes.
func nodeView(record store.Node, reported *NodeState, release string) Node {
	state := StateOffline
	if record.SessionID != "" {
		state = StateOnline
	}
	view := Node{
		ID: record.ID, Name: record.Name, State: state, CreatedAt: record.CreatedAt,
		EnrolledAt: record.EnrolledAt, LastSeenAt: record.LastSeenAt,
		Hostname: record.Hostname, OS: record.OS, Arch: record.Arch, BuildVersion: record.BuildVersion,
		Tags: record.Settings.Tags,
	}
	if reported != nil {
		runtime := runtimeStatusView(reported.Report.GetRuntime())
		view.ReportedAt, view.PublicIP, view.Runtime = &reported.ReportedAt, reported.Report.GetHost().GetPublicIp(), &runtime
		view.LastUpdate = updateAttemptView(reported.Report.GetLastUpdate())
	}
	view.UpdateAvailable = release != "" && record.BuildVersion != "" && record.BuildVersion != release
	return view
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

// NodeSettings are the panel's settings for one node. Revision guards saves
// against concurrent changes.
type NodeSettings struct {
	Revision int64 `json:"revision"`
	// PublicHost is the host subscriptions publish for the node's inbounds;
	// empty uses the public IP the node reports.
	PublicHost string              `json:"public_host"`
	Traffic    NodeTrafficSettings `json:"traffic"`
	// Tags are up to 16 names of 1 to 32 lowercase letters, digits, hyphens
	// and underscores, stored sorted and distinct.
	Tags []string `json:"tags"`
}

// NodeTrafficSettings is a node's own traffic quota and period: a null quota
// is unlimited, and each period of PeriodMonths starts at 00:00 UTC on
// ResetDay.
type NodeTrafficSettings struct {
	QuotaGiB     *int64 `json:"quota_gib"`
	PeriodMonths int    `json:"period_months"`
	ResetDay     int    `json:"reset_day"`
}

func (service *Service) NodeSettings(ctx context.Context, id string) (NodeSettings, error) {
	record, err := service.database.Node(ctx, id)
	if err != nil {
		return NodeSettings{}, err
	}
	return nodeSettingsView(record.Settings), nil
}

// SaveNodeSettings replaces the node's settings if they are still at
// value.Revision. Subscriptions, periods and quota status follow at once;
// recorded traffic is unchanged.
func (service *Service) SaveNodeSettings(ctx context.Context, id string, value NodeSettings) (result NodeSettings, err error) {
	defer func() {
		service.expireStatus()
		service.records.RecordOperation(ctx, "node.settings.save", "Node settings save", err, telemetry.OperationLogContext{NodeID: id})
	}()
	previous, previousErr := service.database.Node(ctx, id)
	record, err := service.database.SaveNodeSettings(ctx, id, value.Revision, store.NodeSettings{
		PublicHost: value.PublicHost,
		Traffic:    store.NodeTrafficSettings{QuotaGiB: value.Traffic.QuotaGiB, PeriodMonths: value.Traffic.PeriodMonths, ResetDay: value.Traffic.ResetDay},
		Tags:       value.Tags,
	})
	if err != nil {
		return NodeSettings{}, err
	}
	if previousErr != nil || previous.Settings.PublicHost != record.Settings.PublicHost {
		service.publicationChanged(ctx)
	}
	return nodeSettingsView(record.Settings), nil
}

// publicationChanged reports a change to what a node's subscription inbounds
// derive from.
func (service *Service) publicationChanged(ctx context.Context) {
	if service.published != nil {
		service.published(ctx)
	}
}

func nodeSettingsView(value store.NodeSettings) NodeSettings {
	return NodeSettings{Revision: value.Revision, PublicHost: value.PublicHost, Traffic: NodeTrafficSettings{
		QuotaGiB: value.Traffic.QuotaGiB, PeriodMonths: value.Traffic.PeriodMonths, ResetDay: value.Traffic.ResetDay,
	}, Tags: value.Tags}
}
