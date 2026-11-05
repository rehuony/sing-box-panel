// SPDX-License-Identifier: GPL-3.0-or-later

package fleet

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/rehuony/sing-box-panel/internal/nodeapi"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/corecache"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/nodeconfig"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/telemetry"
)

// InstallTimeout bounds installing a core, which includes the transfer.
const InstallTimeout = 10 * time.Minute

var errUnexpectedResult = errors.New("the node answered with an unexpected result")

// RuntimeStatus reads the node's runtime state.
func (service *Service) RuntimeStatus(ctx context.Context, nodeID string) (RuntimeStatus, error) {
	response, err := service.runtimeCommand(ctx, nodeID, &nodeapi.Command_GetRuntimeStatus{GetRuntimeStatus: &nodeapi.GetRuntimeStatus{}})
	return response.Status, err
}

// StartRuntime starts the node's core with its saved configuration.
func (service *Service) StartRuntime(ctx context.Context, nodeID string) (result RuntimeStatus, err error) {
	defer service.recordCommand(ctx, "runtime.start", "Core start", nodeID, &err)
	configuration, err := service.enabledConfiguration(ctx, nodeID)
	if err != nil {
		return RuntimeStatus{}, err
	}
	response, err := service.runtimeCommand(ctx, nodeID, &nodeapi.Command_StartRuntime{StartRuntime: &nodeapi.StartRuntime{Configuration: configuration}})
	return response.Status, err
}

// StopRuntime stops the node's core.
func (service *Service) StopRuntime(ctx context.Context, nodeID string) (result RuntimeStatus, err error) {
	defer service.recordCommand(ctx, "runtime.stop", "Core stop", nodeID, &err)
	response, err := service.runtimeCommand(ctx, nodeID, &nodeapi.Command_StopRuntime{StopRuntime: &nodeapi.StopRuntime{}})
	return response.Status, err
}

// RestartRuntime checks the saved configuration and replaces the node's core
// process.
func (service *Service) RestartRuntime(ctx context.Context, nodeID string) (result RuntimeStatus, err error) {
	defer service.recordCommand(ctx, "runtime.restart", "Core restart", nodeID, &err)
	configuration, err := service.enabledConfiguration(ctx, nodeID)
	if err != nil {
		return RuntimeStatus{}, err
	}
	response, err := service.runtimeCommand(ctx, nodeID, &nodeapi.Command_RestartRuntime{RestartRuntime: &nodeapi.RestartRuntime{Configuration: configuration}})
	return response.Status, err
}

// RollbackRuntime returns the node to bundleID, or to its rollback bundle.
func (service *Service) RollbackRuntime(ctx context.Context, nodeID, bundleID string) (result RuntimeStatus, err error) {
	defer service.recordCommand(ctx, "runtime.rollback", "Core rollback", nodeID, &err)
	response, err := service.runtimeCommand(ctx, nodeID, &nodeapi.Command_RollbackRuntime{RollbackRuntime: &nodeapi.RollbackRuntime{BundleId: bundleID}})
	return response.Status, err
}

// EnableCore selects a core on the node, checking the saved configuration
// with it.
func (service *Service) EnableCore(ctx context.Context, nodeID, artifactID string) (result RuntimeStatus, err error) {
	defer service.recordCommand(ctx, "core.enable", "Core selection", nodeID, &err)
	configuration, err := service.configuration(ctx, nodeID, service.forCore(ctx, artifactID))
	if err != nil {
		return RuntimeStatus{}, err
	}
	response, err := service.runtimeCommand(ctx, nodeID, &nodeapi.Command_EnableCore{EnableCore: &nodeapi.EnableCore{Configuration: configuration, ArtifactId: artifactID}})
	return response.Status, err
}

// DisableCore stops the node's core and clears its selection.
func (service *Service) DisableCore(ctx context.Context, nodeID, artifactID string) (result RuntimeStatus, err error) {
	defer service.recordCommand(ctx, "core.disable", "Core deselection", nodeID, &err)
	response, err := service.runtimeCommand(ctx, nodeID, &nodeapi.Command_DisableCore{DisableCore: &nodeapi.DisableCore{ArtifactId: artifactID}})
	return response.Status, err
}

// CompileConfiguration checks the node's saved configuration with a core and
// records the checked startup artifact on the node.
func (service *Service) CompileConfiguration(ctx context.Context, nodeID, artifactID string) (result ConfigurationCompile, err error) {
	defer service.recordCommand(ctx, "configuration.check", "Configuration check", nodeID, &err)
	support, err := service.configs.ConfigurationSupport(ctx, artifactID)
	if err != nil {
		return ConfigurationCompile{}, err
	}
	configuration, err := service.configuration(ctx, nodeID, service.forCore(ctx, artifactID))
	if err != nil {
		return ConfigurationCompile{}, err
	}
	outcome, err := service.run(ctx, nodeID, &nodeapi.Command_CheckConfiguration{CheckConfiguration: &nodeapi.CheckConfiguration{
		Configuration: configuration, ArtifactId: artifactID,
	}})
	if err != nil {
		return ConfigurationCompile{}, err
	}
	startup := outcome.GetCheck().GetStartup()
	if startup == nil {
		return ConfigurationCompile{}, errUnexpectedResult
	}
	return ConfigurationCompile{Support: support, Artifact: CompiledConfigurationArtifact{
		ID: startup.GetId(), CanonicalRevisionID: startup.GetRevisionId(), ExactCoreVersion: startup.GetExactCoreVersion(),
		CoreArtifactID: startup.GetCoreArtifactId(), ConfigSHA256: startup.GetConfigSha256(), State: startupStateView(startup.GetState()),
	}}, nil
}

// CheckStartup checks a recorded startup artifact again.
func (service *Service) CheckStartup(ctx context.Context, nodeID, startupID string) (result StartupArtifactSummary, err error) {
	defer service.recordCommand(ctx, "runtime.check", "Startup check", nodeID, &err)
	outcome, err := service.run(ctx, nodeID, &nodeapi.Command_CheckStartup{CheckStartup: &nodeapi.CheckStartup{StartupArtifactId: startupID}})
	if err != nil {
		return StartupArtifactSummary{}, err
	}
	startup := outcome.GetCheck().GetStartup()
	if startup == nil {
		return StartupArtifactSummary{}, errUnexpectedResult
	}
	return startupView(startup), nil
}

// ActivateStartup applies a checked startup artifact with a monitoring tier.
func (service *Service) ActivateStartup(ctx context.Context, nodeID, startupID, tier string) (result RuntimeResponse, err error) {
	defer service.recordCommand(ctx, "runtime.activate", "Startup activation", nodeID, &err)
	return service.runtimeCommand(ctx, nodeID, &nodeapi.Command_ActivateStartup{ActivateStartup: &nodeapi.ActivateStartup{
		StartupArtifactId: startupID, MonitoringTier: monitoringTierProto(tier),
	}})
}

// ListStartupArtifacts lists the node's startup artifacts.
func (service *Service) ListStartupArtifacts(ctx context.Context, nodeID string, request StartupArtifactListRequest) (StartupArtifactPage, error) {
	command := &nodeapi.ListStartupArtifacts{
		RevisionId: request.CanonicalRevisionID, ExactCoreVersion: request.ExactCoreVersion,
		CoreArtifactId: request.CoreArtifactID, State: startupStateProto(request.State), Limit: uint32(max(0, request.Limit)),
	}
	if request.Cursor != nil {
		command.BeforeTime, command.BeforeId = timestamppb.New(request.Cursor.CreatedAt), request.Cursor.ID
	}
	outcome, err := service.run(ctx, nodeID, &nodeapi.Command_ListStartupArtifacts{ListStartupArtifacts: command})
	if err != nil {
		return StartupArtifactPage{}, err
	}
	listed := outcome.GetStartupArtifacts()
	page := StartupArtifactPage{Items: make([]StartupArtifactSummary, 0, len(listed.GetItems()))}
	for _, item := range listed.GetItems() {
		page.Items = append(page.Items, startupView(item))
	}
	if listed.GetNextBeforeTime() != nil {
		page.Next = &StartupArtifactCursor{CreatedAt: listed.GetNextBeforeTime().AsTime(), ID: listed.GetNextBeforeId()}
	}
	return page, nil
}

// InstallCore installs a cached core on the node, which fetches it from the
// panel with a short-lived grant.
func (service *Service) InstallCore(ctx context.Context, nodeID string, artifact store.CachedArtifact) (result corecache.CoreArtifact, err error) {
	defer service.recordCommand(ctx, "core.install", "Core installation", nodeID, &err)
	identity, err := artifactIdentity(artifact)
	if err != nil {
		return corecache.CoreArtifact{}, err
	}
	grant, err := service.grants.issue(service.random, nodeID, artifact.ID, service.now().Add(InstallTimeout))
	if err != nil {
		return corecache.CoreArtifact{}, err
	}
	defer service.grants.revoke(grant)
	ctx, cancel := context.WithTimeout(ctx, InstallTimeout)
	defer cancel()
	outcome, err := service.run(ctx, nodeID, &nodeapi.Command_InstallCore{InstallCore: &nodeapi.InstallCore{
		Artifact: identity, FetchGrant: grant, Size: artifact.Size,
	}})
	if err != nil {
		return corecache.CoreArtifact{}, err
	}
	installed := outcome.GetCore().GetInstalled()
	if len(installed) != 1 {
		return corecache.CoreArtifact{}, errUnexpectedResult
	}
	return coreView(installed[0]), nil
}

// RemoveCore removes an installed core from the node.
func (service *Service) RemoveCore(ctx context.Context, nodeID, artifactID string) (err error) {
	defer service.recordCommand(ctx, "core.remove", "Core removal", nodeID, &err)
	_, err = service.run(ctx, nodeID, &nodeapi.Command_RemoveCore{RemoveCore: &nodeapi.RemoveCore{ArtifactId: artifactID}})
	return err
}

// ListCores lists the cores installed on the node.
func (service *Service) ListCores(ctx context.Context, nodeID string, filter corecache.CoreArtifactListFilter) (corecache.CoreArtifactPage, error) {
	command := &nodeapi.ListCores{
		ExactVersion: filter.ExactVersion, Arch: filter.Architecture, Variant: filter.Variant, Limit: uint32(max(0, filter.Limit)),
		SourceKind: sourceKindProto(filter.SourceKind),
	}
	if filter.Cursor != nil {
		command.BeforeTime, command.BeforeId = timestamppb.New(filter.Cursor.CreatedAt), filter.Cursor.ID
	}
	outcome, err := service.run(ctx, nodeID, &nodeapi.Command_ListCores{ListCores: command})
	if err != nil {
		return corecache.CoreArtifactPage{}, err
	}
	listed := outcome.GetCore()
	page := corecache.CoreArtifactPage{Items: make([]corecache.CoreArtifact, 0, len(listed.GetInstalled()))}
	for _, core := range listed.GetInstalled() {
		page.Items = append(page.Items, coreView(core))
	}
	if listed.GetNextBeforeTime() != nil {
		page.Next = &corecache.CoreArtifactCursor{CreatedAt: listed.GetNextBeforeTime().AsTime(), ID: listed.GetNextBeforeId()}
	}
	return page, nil
}

// Core returns one core installed on the node.
func (service *Service) Core(ctx context.Context, nodeID, artifactID string) (corecache.CoreArtifact, error) {
	outcome, err := service.run(ctx, nodeID, &nodeapi.Command_ListCores{ListCores: &nodeapi.ListCores{Id: artifactID}})
	if err != nil {
		return corecache.CoreArtifact{}, err
	}
	installed := outcome.GetCore().GetInstalled()
	if len(installed) != 1 {
		return corecache.CoreArtifact{}, errUnexpectedResult
	}
	return coreView(installed[0]), nil
}

// Browse lists one directory of the node's filesystem.
func (service *Service) Browse(ctx context.Context, nodeID string, query PathQuery) (PathPage, error) {
	outcome, err := service.run(ctx, nodeID, &nodeapi.Command_BrowseFilesystem{BrowseFilesystem: &nodeapi.BrowseFilesystem{
		Path: query.Path, Search: query.Search, ShowHidden: query.ShowHidden,
		Offset: uint32(max(0, query.Offset)), Limit: uint32(max(0, query.Limit)),
	}})
	if err != nil {
		return PathPage{}, err
	}
	browse := outcome.GetBrowse()
	page := PathPage{
		Path: browse.GetPath(), Parent: browse.GetParent(), RequestedPath: browse.GetRequestedPath(), Fallback: browse.GetFallback(),
		Total: int(browse.GetTotal()), Offset: int(browse.GetOffset()), Limit: int(browse.GetLimit()),
		Items: make([]PathEntry, 0, len(browse.GetItems())),
	}
	for _, item := range browse.GetItems() {
		page.Items = append(page.Items, PathEntry{Name: item.GetName(), Path: item.GetPath(), Kind: item.GetKind(), Symlink: item.GetSymlink(), Available: item.GetAvailable()})
	}
	return page, nil
}

// Resolve resolves a path on the node for a field of the given mode.
func (service *Service) Resolve(ctx context.Context, nodeID, path, mode string) (PathSelection, error) {
	outcome, err := service.run(ctx, nodeID, &nodeapi.Command_ResolveFilesystemPath{ResolveFilesystemPath: &nodeapi.ResolveFilesystemPath{
		Path: path, Mode: filesystemModeProto(mode),
	}})
	if err != nil {
		return PathSelection{}, err
	}
	resolved := outcome.GetResolve()
	return PathSelection{Path: resolved.GetPath(), Parent: resolved.GetParent(), Kind: resolved.GetKind(), Exists: resolved.GetExists(), Symlink: resolved.GetSymlink()}, nil
}

// CoreLogFiles lists the node's core log files.
func (service *Service) CoreLogFiles(ctx context.Context, nodeID string) ([]CoreLogFile, error) {
	outcome, err := service.run(ctx, nodeID, &nodeapi.Command_ListCoreLogs{ListCoreLogs: &nodeapi.ListCoreLogs{}})
	if err != nil {
		return nil, err
	}
	files := make([]CoreLogFile, 0, len(outcome.GetCoreLog().GetFiles()))
	for _, file := range outcome.GetCoreLog().GetFiles() {
		files = append(files, CoreLogFile{Name: file.GetName(), Size: file.GetSize(), UpdatedAt: file.GetUpdatedAt().AsTime(), Deletable: file.GetDeletable()})
	}
	return files, nil
}

// ReadCoreLog reads a node's core log file from offset.
func (service *Service) ReadCoreLog(ctx context.Context, nodeID, file string, offset int64, generation string) (CoreLogChunk, error) {
	outcome, err := service.run(ctx, nodeID, &nodeapi.Command_ReadCoreLog{ReadCoreLog: &nodeapi.ReadCoreLog{File: file, Offset: offset, Generation: generation}})
	if err != nil {
		return CoreLogChunk{}, err
	}
	chunk := outcome.GetCoreLog().GetChunk()
	if chunk == nil {
		return CoreLogChunk{}, errUnexpectedResult
	}
	return CoreLogChunk{File: chunk.GetFile(), Text: chunk.GetText(), NextOffset: chunk.GetNextOffset(), Size: chunk.GetSize(), Generation: chunk.GetGeneration(), Reset: chunk.GetReset_()}, nil
}

// DeleteCoreLog deletes an older core log file on the node.
func (service *Service) DeleteCoreLog(ctx context.Context, nodeID, file string) (err error) {
	defer service.recordCommand(ctx, "core.log.delete", "Core log file deletion", nodeID, &err)
	_, err = service.run(ctx, nodeID, &nodeapi.Command_DeleteCoreLog{DeleteCoreLog: &nodeapi.DeleteCoreLog{File: file}})
	return err
}

// ClearCoreLog empties a core log file on the node.
func (service *Service) ClearCoreLog(ctx context.Context, nodeID, file string) (err error) {
	defer service.recordCommand(ctx, "core.log.clear", "Core log clearing", nodeID, &err)
	_, err = service.run(ctx, nodeID, &nodeapi.Command_ClearCoreLog{ClearCoreLog: &nodeapi.ClearCoreLog{File: file}})
	return err
}

func (service *Service) runtimeCommand(ctx context.Context, nodeID string, body nodeapi.CommandBody) (RuntimeResponse, error) {
	outcome, err := service.run(ctx, nodeID, body)
	if err != nil {
		return RuntimeResponse{}, err
	}
	runtime := outcome.GetRuntime()
	if runtime.GetStatus() == nil {
		return RuntimeResponse{}, errUnexpectedResult
	}
	return runtimeView(runtime), nil
}

// enabledConfiguration returns the node's saved revision for the core the
// node reports enabled, checked against that core's exact Schema. Without an
// enabled core the node rejects the command itself.
func (service *Service) enabledConfiguration(ctx context.Context, nodeID string) (*nodeapi.ConfigurationRevision, error) {
	status, err := service.runtimeCommand(ctx, nodeID, &nodeapi.Command_GetRuntimeStatus{GetRuntimeStatus: &nodeapi.GetRuntimeStatus{}})
	if err != nil {
		return nil, err
	}
	enabled := status.Status.EnabledCore
	return service.configuration(ctx, nodeID, func(revision store.CanonicalRevision) error {
		if enabled == nil {
			return nil
		}
		return nodeconfig.ValidateForVersion(revision, enabled.ExactCoreVersion)
	})
}

// forCore validates a revision against the exact Schema of a cached core.
func (service *Service) forCore(ctx context.Context, artifactID string) func(store.CanonicalRevision) error {
	return func(revision store.CanonicalRevision) error {
		return service.configs.ValidateForCore(ctx, revision, artifactID)
	}
}

// configuration returns the node's saved revision as a command carries it,
// once validate accepts it.
func (service *Service) configuration(ctx context.Context, nodeID string, validate func(store.CanonicalRevision) error) (*nodeapi.ConfigurationRevision, error) {
	revision, fileRevision, err := service.configs.CurrentRevision(ctx, nodeID)
	if err != nil {
		return nil, err
	}
	if err := validate(revision); err != nil {
		return nil, err
	}
	digest := sha256.Sum256(revision.Document)
	return &nodeapi.ConfigurationRevision{
		RevisionId: revision.ID, Revision: fileRevision, Sha256: hex.EncodeToString(digest[:]), Content: revision.Document,
	}, nil
}

// recordCommand records the outcome of a management command for a node.
func (service *Service) recordCommand(ctx context.Context, code, message, nodeID string, err *error) {
	service.records.RecordOperation(ctx, code, message, *err, telemetry.OperationLogContext{NodeID: nodeID})
}

func runtimeView(outcome *nodeapi.RuntimeOutcome) RuntimeResponse {
	view := RuntimeResponse{Status: runtimeStatusView(outcome.GetStatus())}
	if activation := outcome.GetActivation(); activation != nil {
		view.Activation = &ActivationSummary{
			StartupArtifactID: activation.GetStartupArtifactId(), CanonicalRevisionID: activation.GetRevisionId(),
			ExactCoreVersion: activation.GetExactCoreVersion(), CoreArtifactID: activation.GetCoreArtifactId(),
			ConfigSHA256: activation.GetConfigSha256(), ActivationBundleID: activation.GetActivationBundleId(),
			ActivationSHA256: activation.GetActivationSha256(), MonitoringTier: monitoringTierView(activation.GetMonitoringTier()),
		}
	}
	if startup := outcome.GetStartup(); startup != nil {
		summary := startupView(startup)
		view.Startup = &summary
	}
	return view
}

func runtimeStatusView(status *nodeapi.RuntimeStatus) RuntimeStatus {
	view := RuntimeStatus{
		LoadedCanonicalRevisionID: status.GetLoadedRevisionId(), DesiredRunning: status.GetDesiredRunning(),
		DesiredBundleID: status.GetDesiredBundleId(), AppliedBundleID: status.GetAppliedBundleId(),
		RollbackBundleID: status.GetRollbackBundleId(), TargetGeneration: status.GetTargetGeneration(),
		ObservationState: status.GetObservationState(),
	}
	if status.GetEnabledCoreArtifactId() != "" {
		view.EnabledCore = &EnabledCore{CoreArtifactID: status.GetEnabledCoreArtifactId(), ExactCoreVersion: status.GetEnabledExactCoreVersion()}
	}
	if running := status.GetRunning(); running != nil {
		view.Running = &RunningCore{
			PID: int(running.GetPid()), ProcessStartToken: running.GetProcessStartToken(), ExactCoreVersion: running.GetExactCoreVersion(),
			CoreArtifactID: running.GetCoreArtifactId(), ArchiveSHA256: running.GetArchiveSha256(), BinarySHA256: running.GetBinarySha256(),
			ActivationBundleID: running.GetActivationBundleId(), StartedAt: running.GetStartedAt().AsTime(),
		}
	}
	return view
}

func startupView(startup *nodeapi.StartupArtifact) StartupArtifactSummary {
	summary := StartupArtifactSummary{
		ID: startup.GetId(), CanonicalRevisionID: startup.GetRevisionId(), ExactCoreVersion: startup.GetExactCoreVersion(),
		CoreArtifactID: startup.GetCoreArtifactId(), ConfigSHA256: startup.GetConfigSha256(), State: startupStateView(startup.GetState()),
		CreatedAt: startup.GetCreatedAt().AsTime(),
	}
	if startup.GetCheckedAt() != nil {
		checked := startup.GetCheckedAt().AsTime()
		summary.CheckedAt = &checked
	}
	return summary
}

func startupStateView(state nodeapi.StartupArtifactState) string {
	switch state {
	case nodeapi.StartupArtifactState_STARTUP_ARTIFACT_STATE_PENDING:
		return "pending"
	case nodeapi.StartupArtifactState_STARTUP_ARTIFACT_STATE_READY:
		return "ready"
	case nodeapi.StartupArtifactState_STARTUP_ARTIFACT_STATE_FAILED:
		return "failed"
	}
	return ""
}

func startupStateProto(state string) nodeapi.StartupArtifactState {
	switch state {
	case "pending":
		return nodeapi.StartupArtifactState_STARTUP_ARTIFACT_STATE_PENDING
	case "ready":
		return nodeapi.StartupArtifactState_STARTUP_ARTIFACT_STATE_READY
	case "failed":
		return nodeapi.StartupArtifactState_STARTUP_ARTIFACT_STATE_FAILED
	}
	return nodeapi.StartupArtifactState_STARTUP_ARTIFACT_STATE_UNSPECIFIED
}

func monitoringTierView(tier nodeapi.MonitoringTier) string {
	switch tier {
	case nodeapi.MonitoringTier_MONITORING_TIER_LIMITED:
		return "limited"
	case nodeapi.MonitoringTier_MONITORING_TIER_PROCESS_ONLY:
		return "process_only"
	}
	return ""
}

func monitoringTierProto(tier string) nodeapi.MonitoringTier {
	switch tier {
	case "limited":
		return nodeapi.MonitoringTier_MONITORING_TIER_LIMITED
	case "process_only":
		return nodeapi.MonitoringTier_MONITORING_TIER_PROCESS_ONLY
	}
	return nodeapi.MonitoringTier_MONITORING_TIER_UNSPECIFIED
}

func filesystemModeProto(mode string) nodeapi.FilesystemMode {
	switch mode {
	case "file":
		return nodeapi.FilesystemMode_FILESYSTEM_MODE_FILE
	case "directory":
		return nodeapi.FilesystemMode_FILESYSTEM_MODE_DIRECTORY
	case "output-file":
		return nodeapi.FilesystemMode_FILESYSTEM_MODE_OUTPUT_FILE
	case "socket":
		return nodeapi.FilesystemMode_FILESYSTEM_MODE_SOCKET
	}
	return nodeapi.FilesystemMode_FILESYSTEM_MODE_UNSPECIFIED
}

func sourceKindProto(kind string) nodeapi.ArtifactSourceKind {
	switch kind {
	case store.CachedArtifactOfficial:
		return nodeapi.ArtifactSourceKind_ARTIFACT_SOURCE_KIND_OFFICIAL
	case store.CachedArtifactUserVerified:
		return nodeapi.ArtifactSourceKind_ARTIFACT_SOURCE_KIND_USER_VERIFIED
	}
	return nodeapi.ArtifactSourceKind_ARTIFACT_SOURCE_KIND_UNSPECIFIED
}

func coreView(core *nodeapi.InstalledCore) corecache.CoreArtifact {
	identity := core.GetArtifact()
	view := corecache.CoreArtifact{
		ID: identity.GetId(), ExactVersion: identity.GetReportedVersion(), OperatingSystem: identity.GetOs(),
		Architecture: identity.GetArch(), Variant: identity.GetVariant(), UserSource: identity.GetUserSource(),
		RepositoryID: identity.GetRepositoryId(), ReleaseID: identity.GetReleaseId(), AssetID: identity.GetAssetId(),
		ArchiveSHA256: identity.GetSha256(), BinarySHA256: core.GetBinarySha256(), BinaryPath: core.GetBinaryPath(),
		ReportedVersion: identity.GetReportedVersion(), FeatureFingerprint: json.RawMessage(core.GetFeatureFingerprint()),
		CreatedAt: core.GetInstalledAt().AsTime(),
	}
	if !json.Valid(view.FeatureFingerprint) {
		view.FeatureFingerprint = json.RawMessage(`{}`)
	}
	switch identity.GetSourceKind() {
	case nodeapi.ArtifactSourceKind_ARTIFACT_SOURCE_KIND_OFFICIAL:
		view.SourceKind = store.CachedArtifactOfficial
	case nodeapi.ArtifactSourceKind_ARTIFACT_SOURCE_KIND_USER_VERIFIED:
		view.SourceKind = store.CachedArtifactUserVerified
	}
	return view
}

func artifactIdentity(artifact store.CachedArtifact) (*nodeapi.ArtifactIdentity, error) {
	if _, err := corecache.Identity(artifact); err != nil {
		return nil, err
	}
	return &nodeapi.ArtifactIdentity{
		Id: artifact.ID, SourceKind: sourceKindProto(artifact.SourceKind),
		RepositoryId: artifact.RepositoryID, ReleaseId: artifact.ReleaseID, AssetId: artifact.AssetID, UserSource: artifact.UserSource,
		Sha256: artifact.SHA256, Os: artifact.OS, Arch: artifact.Arch, Variant: artifact.Variant, ReportedVersion: artifact.ExactVersion,
	}, nil
}
