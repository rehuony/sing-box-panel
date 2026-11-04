// SPDX-License-Identifier: GPL-3.0-or-later

package agent

import (
	"context"
	"errors"
	"io"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/rehuony/sing-box-panel/internal/coreartifact"
	"github.com/rehuony/sing-box-panel/internal/nodeapi"
	"github.com/rehuony/sing-box-panel/internal/sing-box-node/artifacts"
	"github.com/rehuony/sing-box-panel/internal/sing-box-node/corelogs"
	"github.com/rehuony/sing-box-panel/internal/sing-box-node/host"
	"github.com/rehuony/sing-box-panel/internal/sing-box-node/service"
	"github.com/rehuony/sing-box-panel/internal/sing-box-node/store"
)

var (
	errConfigurationRequired = errors.New("the command carries no configuration")
	errArtifactIdentity      = errors.New("the installed core does not match the artifact identity the panel sent")
)

// executor carries out panel commands with the node runtime.
type executor struct {
	node     *service.Node
	database *store.Store
	// fetch streams one cached core artifact from the panel.
	fetch func(ctx context.Context, grant, artifactID string, destination io.Writer) (int64, error)
	now   func() time.Time
}

// execute runs command and returns its result. A command that changes state
// records its result, and a repeated request ID returns that result instead
// of running the command again.
func (executor *executor) execute(ctx context.Context, command *nodeapi.Command) *nodeapi.Result {
	requestID := command.GetRequestId()
	if requestID == "" || len(requestID) > 256 {
		return &nodeapi.Result{RequestId: requestID, Outcome: problem("invalid_command", "The command has no valid request ID.")}
	}
	if deadline := command.GetDeadline(); deadline != nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithDeadline(ctx, deadline.AsTime())
		defer cancel()
	}
	recorded := nodeapi.ChangesState(command.GetBody())
	if recorded {
		if raw, ok, err := executor.database.CommandResult(ctx, requestID); err == nil && ok {
			var result nodeapi.Result
			if proto.Unmarshal(raw, &result) == nil {
				return &result
			}
		}
	}
	result := &nodeapi.Result{RequestId: requestID, Outcome: executor.run(ctx, command)}
	if recorded {
		if raw, err := proto.Marshal(result); err == nil {
			_ = executor.database.RecordCommandResult(context.WithoutCancel(ctx), requestID, raw, executor.now().UTC())
		}
	}
	return result
}

func (executor *executor) run(ctx context.Context, command *nodeapi.Command) nodeapi.ResultOutcome {
	node := executor.node
	switch body := command.GetBody().(type) {
	case *nodeapi.Command_CheckConfiguration:
		if err := executor.receive(ctx, body.CheckConfiguration.GetConfiguration()); err != nil {
			return failure(err)
		}
		compiled, err := node.CompileConfiguration(ctx, service.ConfigurationCompileRequest{CoreArtifactID: body.CheckConfiguration.GetArtifactId()})
		if err != nil {
			return failure(err)
		}
		return &nodeapi.Result_Check{Check: &nodeapi.CheckOutcome{Startup: &nodeapi.StartupArtifact{
			Id: compiled.ID, RevisionId: compiled.CanonicalRevisionID, ExactCoreVersion: compiled.ExactCoreVersion,
			CoreArtifactId: compiled.CoreArtifactID, ConfigSha256: compiled.ConfigSHA256, State: startupState(compiled.State),
		}}}
	case *nodeapi.Command_StartRuntime:
		if configuration := body.StartRuntime.GetConfiguration(); configuration != nil {
			if err := executor.receive(ctx, configuration); err != nil {
				return failure(err)
			}
		}
		return runtimeResult(node.StartRuntime(ctx))
	case *nodeapi.Command_StopRuntime:
		return runtimeResult(node.StopRuntime(ctx))
	case *nodeapi.Command_RestartRuntime:
		if err := executor.receive(ctx, body.RestartRuntime.GetConfiguration()); err != nil {
			return failure(err)
		}
		return runtimeResult(node.RestartRuntime(ctx))
	case *nodeapi.Command_RollbackRuntime:
		return runtimeResult(node.RollbackRuntime(ctx, body.RollbackRuntime.GetBundleId()))
	case *nodeapi.Command_EnableCore:
		if err := executor.receive(ctx, body.EnableCore.GetConfiguration()); err != nil {
			return failure(err)
		}
		return runtimeResult(node.EnableCore(ctx, body.EnableCore.GetArtifactId()))
	case *nodeapi.Command_DisableCore:
		return runtimeResult(node.DisableCore(ctx, body.DisableCore.GetArtifactId()))
	case *nodeapi.Command_GetRuntimeStatus:
		return runtimeResult(node.RuntimeStatus(ctx))
	case *nodeapi.Command_CheckStartup:
		startup, err := node.CheckStartup(ctx, body.CheckStartup.GetStartupArtifactId())
		if err != nil {
			return failure(err)
		}
		return &nodeapi.Result_Check{Check: &nodeapi.CheckOutcome{Startup: startupArtifact(startup)}}
	case *nodeapi.Command_ActivateStartup:
		response, err := node.ActivateRuntime(ctx, body.ActivateStartup.GetStartupArtifactId(), monitoringTier(body.ActivateStartup.GetMonitoringTier()))
		if err != nil {
			return failure(err)
		}
		return &nodeapi.Result_Runtime{Runtime: runtimeOutcome(response)}
	case *nodeapi.Command_ListStartupArtifacts:
		return executor.listStartupArtifacts(ctx, body.ListStartupArtifacts)
	case *nodeapi.Command_InstallCore:
		return executor.installCore(ctx, body.InstallCore)
	case *nodeapi.Command_RemoveCore:
		if err := node.RemoveCoreArtifact(ctx, body.RemoveCore.GetArtifactId()); err != nil {
			return failure(err)
		}
		return &nodeapi.Result_Core{Core: &nodeapi.CoreOutcome{}}
	case *nodeapi.Command_ListCores:
		return executor.listCores(ctx, body.ListCores)
	case *nodeapi.Command_BrowseFilesystem:
		request := body.BrowseFilesystem
		page, err := node.ListFilesystemEntries(ctx, host.PathQuery{
			Path: request.GetPath(), Search: request.GetSearch(), ShowHidden: request.GetShowHidden(),
			Offset: int(request.GetOffset()), Limit: int(request.GetLimit()),
		})
		if err != nil {
			return failure(err)
		}
		return &nodeapi.Result_Browse{Browse: browseOutcome(page)}
	case *nodeapi.Command_ResolveFilesystemPath:
		selection, err := node.ResolveFilesystemPath(ctx, body.ResolveFilesystemPath.GetPath(), pathMode(body.ResolveFilesystemPath.GetMode()))
		if err != nil {
			return failure(err)
		}
		return &nodeapi.Result_Resolve{Resolve: &nodeapi.ResolveOutcome{
			Path: selection.Path, Parent: selection.Parent, Kind: selection.Kind, Exists: selection.Exists, Symlink: selection.Symlink,
		}}
	case *nodeapi.Command_ListCoreLogs:
		files, err := node.CoreLogFiles()
		if err != nil {
			return failure(err)
		}
		return &nodeapi.Result_CoreLog{CoreLog: &nodeapi.CoreLogOutcome{Files: coreLogFiles(files)}}
	case *nodeapi.Command_ReadCoreLog:
		request := body.ReadCoreLog
		chunk, err := node.CoreLogContent(request.GetFile(), request.GetOffset(), request.GetGeneration())
		if err != nil {
			return failure(err)
		}
		return &nodeapi.Result_CoreLog{CoreLog: &nodeapi.CoreLogOutcome{Chunk: &nodeapi.CoreLogChunk{
			File: chunk.File, Text: chunk.Text, NextOffset: chunk.NextOffset, Size: chunk.Size, Generation: chunk.Generation, Reset_: chunk.Reset,
		}}}
	case *nodeapi.Command_DeleteCoreLog:
		if err := node.DeleteCoreLogFile(ctx, body.DeleteCoreLog.GetFile()); err != nil {
			return failure(err)
		}
		return &nodeapi.Result_CoreLog{CoreLog: &nodeapi.CoreLogOutcome{}}
	case *nodeapi.Command_ClearCoreLog:
		if err := node.ClearCoreLog(ctx, body.ClearCoreLog.GetFile()); err != nil {
			return failure(err)
		}
		return &nodeapi.Result_CoreLog{CoreLog: &nodeapi.CoreLogOutcome{}}
	}
	return problem("command_unsupported", "This node does not support the command.")
}

// receive records the configuration a command carries as the one to run.
func (executor *executor) receive(ctx context.Context, configuration *nodeapi.ConfigurationRevision) error {
	if configuration == nil {
		return errConfigurationRequired
	}
	_, err := executor.node.ReceiveConfiguration(ctx, configuration.GetRevisionId(), configuration.GetContent(), configuration.GetSha256())
	return err
}

func (executor *executor) installCore(ctx context.Context, request *nodeapi.InstallCore) nodeapi.ResultOutcome {
	artifact := request.GetArtifact()
	var source coreartifact.Source
	var err error
	switch artifact.GetSourceKind() {
	case nodeapi.ArtifactSourceKind_ARTIFACT_SOURCE_KIND_OFFICIAL:
		source, err = coreartifact.NewOfficialSource(artifact.GetRepositoryId(), artifact.GetReleaseId(), artifact.GetAssetId())
	case nodeapi.ArtifactSourceKind_ARTIFACT_SOURCE_KIND_USER_VERIFIED:
		source, err = coreartifact.NewUserSource(artifact.GetUserSource())
	default:
		err = coreartifact.ErrInvalidIdentity
	}
	if err != nil {
		return failure(err)
	}
	digest, err := coreartifact.ParseSHA256(artifact.GetSha256())
	if err != nil {
		return failure(err)
	}
	version, err := coreartifact.ParseExactVersion(artifact.GetReportedVersion())
	if err != nil {
		return failure(err)
	}
	if artifact.GetOs() != string(coreartifact.OperatingSystemLinux) {
		return failure(service.ErrCorePlatformMismatch)
	}
	installed, err := executor.node.InstallCore(ctx, artifacts.Candidate{
		Source: source, SHA256: digest, Size: request.GetSize(), Version: version,
		Architecture: coreartifact.Architecture(artifact.GetArch()), Variant: coreartifact.Variant(artifact.GetVariant()),
		Write: func(ctx context.Context, destination io.Writer) (int64, error) {
			return executor.fetch(ctx, request.GetFetchGrant(), artifact.GetId(), destination)
		},
	})
	if err != nil {
		return failure(err)
	}
	if installed.ID != artifact.GetId() {
		return failure(errArtifactIdentity)
	}
	return &nodeapi.Result_Core{Core: &nodeapi.CoreOutcome{Installed: []*nodeapi.InstalledCore{installedCore(installed)}}}
}

func (executor *executor) listCores(ctx context.Context, request *nodeapi.ListCores) nodeapi.ResultOutcome {
	if id := request.GetId(); id != "" {
		core, err := executor.database.GetCoreArtifact(ctx, id)
		if err != nil {
			return failure(err)
		}
		return &nodeapi.Result_Core{Core: &nodeapi.CoreOutcome{Installed: []*nodeapi.InstalledCore{installedCore(core)}}}
	}
	filter := store.CoreArtifactListFilter{
		ExactVersion: request.GetExactVersion(), Architecture: request.GetArch(), Variant: request.GetVariant(),
		Limit: int(request.GetLimit()),
	}
	switch request.GetSourceKind() {
	case nodeapi.ArtifactSourceKind_ARTIFACT_SOURCE_KIND_OFFICIAL:
		filter.SourceKind = store.CoreArtifactSourceOfficial
	case nodeapi.ArtifactSourceKind_ARTIFACT_SOURCE_KIND_USER_VERIFIED:
		filter.SourceKind = store.CoreArtifactSourceUserVerified
	}
	if request.GetBeforeTime() != nil {
		filter.Cursor = &store.CreatedAtCursor{CreatedAt: request.GetBeforeTime().AsTime(), ID: request.GetBeforeId()}
	}
	page, err := executor.database.ListCoreArtifacts(ctx, filter)
	if err != nil {
		return failure(err)
	}
	outcome := &nodeapi.CoreOutcome{Installed: make([]*nodeapi.InstalledCore, 0, len(page.Items))}
	for _, core := range page.Items {
		outcome.Installed = append(outcome.Installed, installedCore(core))
	}
	if page.Next != nil {
		outcome.NextBeforeTime, outcome.NextBeforeId = timestamppb.New(page.Next.CreatedAt), page.Next.ID
	}
	return &nodeapi.Result_Core{Core: outcome}
}

func (executor *executor) listStartupArtifacts(ctx context.Context, request *nodeapi.ListStartupArtifacts) nodeapi.ResultOutcome {
	query := service.StartupArtifactListRequest{
		CanonicalRevisionID: request.GetRevisionId(), ExactCoreVersion: request.GetExactCoreVersion(),
		CoreArtifactID: request.GetCoreArtifactId(), State: storeStartupState(request.GetState()), Limit: int(request.GetLimit()),
	}
	if request.GetBeforeTime() != nil {
		query.Cursor = &service.StartupArtifactCursor{CreatedAt: request.GetBeforeTime().AsTime(), ID: request.GetBeforeId()}
	}
	page, err := executor.node.ListStartupArtifacts(ctx, query)
	if err != nil {
		return failure(err)
	}
	outcome := &nodeapi.StartupArtifactsOutcome{Items: make([]*nodeapi.StartupArtifact, 0, len(page.Items))}
	for _, item := range page.Items {
		outcome.Items = append(outcome.Items, startupArtifact(item))
	}
	if page.Next != nil {
		outcome.NextBeforeTime, outcome.NextBeforeId = timestamppb.New(page.Next.CreatedAt), page.Next.ID
	}
	return &nodeapi.Result_StartupArtifacts{StartupArtifacts: outcome}
}

func failure(err error) nodeapi.ResultOutcome {
	code, message := service.DescribeFailure(err)
	return problem(code, message)
}

func problem(code, detail string) nodeapi.ResultOutcome {
	return &nodeapi.Result_Problem{Problem: &nodeapi.Problem{Code: code, Title: "Node command failed", Detail: detail}}
}

func runtimeResult(status service.RuntimeStatus, err error) nodeapi.ResultOutcome {
	if err != nil {
		return failure(err)
	}
	return &nodeapi.Result_Runtime{Runtime: runtimeOutcome(service.RuntimeResponse{Status: status})}
}

func runtimeOutcome(response service.RuntimeResponse) *nodeapi.RuntimeOutcome {
	status := response.Status
	outcome := &nodeapi.RuntimeOutcome{Status: &nodeapi.RuntimeStatus{
		LoadedRevisionId: status.LoadedCanonicalRevisionID, DesiredRunning: status.DesiredRunning,
		DesiredBundleId: status.DesiredBundleID, AppliedBundleId: status.AppliedBundleID, RollbackBundleId: status.RollbackBundleID,
		TargetGeneration: status.TargetGeneration, ObservationState: status.ObservationState,
	}}
	if status.EnabledCore != nil {
		outcome.Status.EnabledCoreArtifactId = status.EnabledCore.CoreArtifactID
		outcome.Status.EnabledExactCoreVersion = status.EnabledCore.ExactCoreVersion
	}
	if running := status.Running; running != nil {
		outcome.Status.Running = &nodeapi.RunningCore{
			Pid: int64(running.PID), ProcessStartToken: running.ProcessStartToken, ExactCoreVersion: running.ExactCoreVersion,
			CoreArtifactId: running.CoreArtifactID, ArchiveSha256: running.ArchiveSHA256, BinarySha256: running.BinarySHA256,
			ActivationBundleId: running.ActivationBundleID, StartedAt: timestamppb.New(running.StartedAt),
		}
	}
	if activation := response.Activation; activation != nil {
		outcome.Activation = &nodeapi.Activation{
			StartupArtifactId: activation.StartupArtifactID, RevisionId: activation.CanonicalRevisionID,
			ExactCoreVersion: activation.ExactCoreVersion, CoreArtifactId: activation.CoreArtifactID,
			ConfigSha256: activation.ConfigSHA256, ActivationBundleId: activation.ActivationBundleID,
			ActivationSha256: activation.ActivationSHA256, MonitoringTier: monitoringTierProto(activation.MonitoringTier),
		}
	}
	if response.Startup != nil {
		outcome.Startup = startupArtifact(*response.Startup)
	}
	return outcome
}

func startupArtifact(summary service.StartupArtifactSummary) *nodeapi.StartupArtifact {
	artifact := &nodeapi.StartupArtifact{
		Id: summary.ID, RevisionId: summary.CanonicalRevisionID, ExactCoreVersion: summary.ExactCoreVersion,
		CoreArtifactId: summary.CoreArtifactID, ConfigSha256: summary.ConfigSHA256, State: startupState(summary.State),
		CreatedAt: timestamppb.New(summary.CreatedAt),
	}
	if summary.CheckedAt != nil {
		artifact.CheckedAt = timestamppb.New(*summary.CheckedAt)
	}
	return artifact
}

func startupState(state store.StartupArtifactState) nodeapi.StartupArtifactState {
	switch state {
	case store.StartupArtifactPending:
		return nodeapi.StartupArtifactState_STARTUP_ARTIFACT_STATE_PENDING
	case store.StartupArtifactReady:
		return nodeapi.StartupArtifactState_STARTUP_ARTIFACT_STATE_READY
	case store.StartupArtifactFailed:
		return nodeapi.StartupArtifactState_STARTUP_ARTIFACT_STATE_FAILED
	}
	return nodeapi.StartupArtifactState_STARTUP_ARTIFACT_STATE_UNSPECIFIED
}

func storeStartupState(state nodeapi.StartupArtifactState) store.StartupArtifactState {
	switch state {
	case nodeapi.StartupArtifactState_STARTUP_ARTIFACT_STATE_PENDING:
		return store.StartupArtifactPending
	case nodeapi.StartupArtifactState_STARTUP_ARTIFACT_STATE_READY:
		return store.StartupArtifactReady
	case nodeapi.StartupArtifactState_STARTUP_ARTIFACT_STATE_FAILED:
		return store.StartupArtifactFailed
	}
	return ""
}

func monitoringTier(tier nodeapi.MonitoringTier) store.MonitoringTier {
	if tier == nodeapi.MonitoringTier_MONITORING_TIER_PROCESS_ONLY {
		return store.MonitoringProcessOnly
	}
	if tier == nodeapi.MonitoringTier_MONITORING_TIER_LIMITED {
		return store.MonitoringLimited
	}
	return ""
}

func monitoringTierProto(tier store.MonitoringTier) nodeapi.MonitoringTier {
	switch tier {
	case store.MonitoringLimited:
		return nodeapi.MonitoringTier_MONITORING_TIER_LIMITED
	case store.MonitoringProcessOnly:
		return nodeapi.MonitoringTier_MONITORING_TIER_PROCESS_ONLY
	}
	return nodeapi.MonitoringTier_MONITORING_TIER_UNSPECIFIED
}

func installedCore(core store.CoreArtifact) *nodeapi.InstalledCore {
	identity := &nodeapi.ArtifactIdentity{
		Id: core.ID, UserSource: core.UserSource, Sha256: core.ArchiveSHA256,
		RepositoryId: core.RepositoryID, ReleaseId: core.ReleaseID, AssetId: core.AssetID,
		Os: core.OperatingSystem, Arch: core.Architecture, Variant: core.Variant, ReportedVersion: core.ReportedVersion,
	}
	switch core.SourceKind {
	case store.CoreArtifactSourceOfficial:
		identity.SourceKind = nodeapi.ArtifactSourceKind_ARTIFACT_SOURCE_KIND_OFFICIAL
	case store.CoreArtifactSourceUserVerified:
		identity.SourceKind = nodeapi.ArtifactSourceKind_ARTIFACT_SOURCE_KIND_USER_VERIFIED
	}
	return &nodeapi.InstalledCore{
		Artifact: identity, BinarySha256: core.BinarySHA256, InstalledAt: timestamppb.New(core.CreatedAt),
		BinaryPath: core.BinaryPath, FeatureFingerprint: string(core.FeatureFingerprint),
	}
}

func pathMode(mode nodeapi.FilesystemMode) host.PathMode {
	switch mode {
	case nodeapi.FilesystemMode_FILESYSTEM_MODE_FILE:
		return host.PathFile
	case nodeapi.FilesystemMode_FILESYSTEM_MODE_DIRECTORY:
		return host.PathDirectory
	case nodeapi.FilesystemMode_FILESYSTEM_MODE_OUTPUT_FILE:
		return host.PathOutputFile
	case nodeapi.FilesystemMode_FILESYSTEM_MODE_SOCKET:
		return host.PathSocket
	}
	return ""
}

func browseOutcome(page host.PathPage) *nodeapi.BrowseOutcome {
	outcome := &nodeapi.BrowseOutcome{
		Path: page.Path, Parent: page.Parent, RequestedPath: page.RequestedPath, Fallback: page.Fallback,
		Total: uint32(page.Total), Offset: uint32(page.Offset), Limit: uint32(page.Limit),
		Items: make([]*nodeapi.FilesystemEntry, 0, len(page.Items)),
	}
	for _, item := range page.Items {
		outcome.Items = append(outcome.Items, &nodeapi.FilesystemEntry{
			Name: item.Name, Path: item.Path, Kind: item.Kind, Symlink: item.Symlink, Available: item.Available,
		})
	}
	return outcome
}

func coreLogFiles(files []corelogs.File) []*nodeapi.CoreLogFile {
	result := make([]*nodeapi.CoreLogFile, 0, len(files))
	for _, file := range files {
		result = append(result, &nodeapi.CoreLogFile{
			Name: file.Name, Size: file.Size, UpdatedAt: timestamppb.New(file.UpdatedAt), Deletable: file.Deletable,
		})
	}
	return result
}
