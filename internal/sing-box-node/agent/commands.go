// SPDX-License-Identifier: GPL-3.0-or-later

package agent

import (
	"context"
	"errors"
	"io"
	"sync"
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
	// slots bounds the commands that run at once.
	slots chan struct{}
	// updates applies UpdateNode with releases fetchRelease receives.
	updates      *updates
	fetchRelease releaseFetcher

	followMu sync.Mutex
	// followUps run after the panel has a command's result.
	followUps map[string]func()
}

// takeFollowUp returns the work to do once the panel has the result of
// requestID, such as restarting into an update.
func (executor *executor) takeFollowUp(requestID string) func() {
	executor.followMu.Lock()
	defer executor.followMu.Unlock()
	followUp := executor.followUps[requestID]
	delete(executor.followUps, requestID)
	return followUp
}

func (executor *executor) updateNode(ctx context.Context, requestID string, request *nodeapi.UpdateNode) nodeapi.ResultOutcome {
	if executor.updates == nil || executor.fetchRelease == nil {
		return problem("update_unsupported", "This node cannot update itself.")
	}
	state, restart, err := executor.updates.apply(ctx, request.GetVersion(), executor.fetchRelease)
	if err != nil {
		return updateFailure(err)
	}
	if restart != nil {
		executor.followMu.Lock()
		if executor.followUps == nil {
			executor.followUps = map[string]func(){}
		}
		executor.followUps[requestID] = restart
		executor.followMu.Unlock()
	}
	return &nodeapi.Result_Update{Update: &nodeapi.UpdateOutcome{State: state, Version: request.GetVersion()}}
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
	select {
	case executor.slots <- struct{}{}:
		defer func() { <-executor.slots }()
	case <-ctx.Done():
		return &nodeapi.Result{RequestId: requestID, Outcome: problem("node_busy", "The node was running too many commands to start this one before its deadline.")}
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
	case *nodeapi.Command_GetCoreSchema:
		schema, err := node.CoreSchema(ctx, body.GetCoreSchema.GetArtifactId(), nodeapi.MaximumCoreSchemaBytes)
		if err != nil {
			return failure(err)
		}
		return &nodeapi.Result_CoreSchema{CoreSchema: &nodeapi.CoreSchemaOutcome{Schema: schema}}
	case *nodeapi.Command_InstallCore:
		return executor.installCore(ctx, body.InstallCore)
	case *nodeapi.Command_UpdateNode:
		return executor.updateNode(ctx, command.GetRequestId(), body.UpdateNode)
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
	return &nodeapi.Result_Runtime{Runtime: runtimeOutcome(status)}
}

func runtimeOutcome(status service.RuntimeStatus) *nodeapi.RuntimeOutcome {
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
