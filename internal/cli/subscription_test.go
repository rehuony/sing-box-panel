// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/settings"
	"github.com/rehuony/sing-box-panel/internal/store"
)

func TestSubscriptionChannelAndSourceCLIEndToEnd(t *testing.T) {
	settingsPath := commandSettingsFixture(t)

	createdChannelOutput := runApplicationCommand(t, settingsPath,
		`{"name":"public","format":"mihomo","public_host":"public.example","config":{"exclude_tags":["private"]},"enabled":true}`,
		"--output", "json", "subscription", "channel", "create", "--file", "-",
	)
	var channel application.SubscriptionChannel
	decodeSubscriptionCLIOutput(t, createdChannelOutput, &channel)
	if channel.ID == "" || channel.Name != "public" || channel.Format != store.SubscriptionFormatMihomo || !channel.Enabled {
		t.Fatalf("created channel = %+v", channel)
	}

	listedChannelOutput := runApplicationCommand(t, settingsPath, "",
		"--output", "jsonl", "subscription", "channel", "list",
	)
	var listedChannel application.SubscriptionChannelSummary
	decodeSubscriptionCLIOutput(t, listedChannelOutput, &listedChannel)
	if listedChannel.ID != channel.ID || bytes.Count(listedChannelOutput, []byte{'\n'}) != 1 {
		t.Fatalf("listed channel = %+v; output=%s", listedChannel, listedChannelOutput)
	}

	shownChannelOutput := runApplicationCommand(t, settingsPath, "",
		"--output", "json", "subscription", "channel", "show", channel.ID,
	)
	var shownChannel application.SubscriptionChannel
	decodeSubscriptionCLIOutput(t, shownChannelOutput, &shownChannel)
	if shownChannel.ID != channel.ID {
		t.Fatalf("shown channel = %+v", shownChannel)
	}

	updatedChannelOutput := runApplicationCommand(t, settingsPath,
		`{"name":"public-loon","format":"loon","public_host":"loon.example","config":{},"enabled":false}`,
		"--output", "json", "subscription", "channel", "update", channel.ID,
		"--file", "-", "--updated-at", formatSubscriptionTime(channel.UpdatedAt),
	)
	var updatedChannel application.SubscriptionChannel
	decodeSubscriptionCLIOutput(t, updatedChannelOutput, &updatedChannel)
	if updatedChannel.Name != "public-loon" || updatedChannel.Format != store.SubscriptionFormatLoon ||
		updatedChannel.Enabled || !updatedChannel.UpdatedAt.After(channel.UpdatedAt) {
		t.Fatalf("updated channel = %+v", updatedChannel)
	}

	_, _, staleErr := executeSubscriptionCLI(
		t, settingsPath,
		`{"name":"stale","format":"loon","public_host":"stale.example","config":{},"enabled":false}`,
		"--output", "json", "subscription", "channel", "update", channel.ID,
		"--file", "-", "--updated-at", formatSubscriptionTime(channel.UpdatedAt),
	)
	assertSubscriptionCLIError(t, staleErr, ErrorConflict, "subscription_conflict")

	createdSourceOutput := runApplicationCommand(t, settingsPath,
		`{"name":"upstream","source_kind":"remote","config":{"url":"https://example.test/sub"},"enabled":true}`,
		"--output", "json", "subscription", "source", "create", "--file", "-",
	)
	var source application.SubscriptionSource
	decodeSubscriptionCLIOutput(t, createdSourceOutput, &source)
	if source.ID == "" || source.SourceKind != store.SubscriptionSourceRemote || source.CurrentVersionID != "" {
		t.Fatalf("created source = %+v", source)
	}

	refreshedSourceOutput := runApplicationCommand(t, settingsPath, "",
		"--output", "json", "subscription", "source", "refresh", source.ID,
	)
	var refreshTask application.Task
	decodeSubscriptionCLIOutput(t, refreshedSourceOutput, &refreshTask)
	if refreshTask.Kind != "subscription-source-refresh" || refreshTask.Status != store.TaskStatusQueued {
		t.Fatalf("refresh task = %+v", refreshTask)
	}
	shownSourceOutput := runApplicationCommand(t, settingsPath, "",
		"--output", "json", "subscription", "source", "show", source.ID,
	)
	var shownSource application.SubscriptionSource
	decodeSubscriptionCLIOutput(t, shownSourceOutput, &shownSource)
	if shownSource.ID != source.ID || shownSource.CurrentVersionID != "" {
		t.Fatalf("shown source = %+v", shownSource)
	}

	updatedSourceOutput := runApplicationCommand(t, settingsPath,
		`{"name":"local-copy","source_kind":"local","config":{},"enabled":false}`,
		"--output", "json", "subscription", "source", "update", source.ID,
		"--file", "-", "--updated-at", formatSubscriptionTime(source.UpdatedAt),
	)
	var updatedSource application.SubscriptionSource
	decodeSubscriptionCLIOutput(t, updatedSourceOutput, &updatedSource)
	if updatedSource.SourceKind != store.SubscriptionSourceLocal || updatedSource.Enabled {
		t.Fatalf("updated source = %+v", updatedSource)
	}

	listedSourcesOutput := runApplicationCommand(t, settingsPath, "",
		"--output", "json", "subscription", "source", "list",
	)
	var listedSources application.SubscriptionSourcePage
	decodeSubscriptionCLIOutput(t, listedSourcesOutput, &listedSources)
	if len(listedSources.Items) != 1 || listedSources.Items[0].ID != source.ID {
		t.Fatalf("listed sources = %+v", listedSources)
	}

	runApplicationCommand(t, settingsPath, "",
		"--output", "json", "subscription", "source", "delete", source.ID,
		"--updated-at", formatSubscriptionTime(updatedSource.UpdatedAt),
	)
	runApplicationCommand(t, settingsPath, "",
		"--output", "json", "subscription", "channel", "delete", channel.ID,
		"--updated-at", formatSubscriptionTime(updatedChannel.UpdatedAt),
	)
}

func TestSubscriptionChannelRenderCLIEndToEnd(t *testing.T) {
	settingsPath := commandSettingsFixture(t)
	canonicalOutput := runApplicationCommand(t, settingsPath,
		`{"schema_version":2,"configuration":{}}`,
		"--output", "json", "config", "replace", "--file", "-", "--base-revision", "none",
	)
	var canonicalSave application.CanonicalSave
	decodeSubscriptionCLIOutput(t, canonicalOutput, &canonicalSave)

	channelOutput := runApplicationCommand(t, settingsPath,
		`{"name":"preview","format":"sing-box","public_host":"preview.example","config":{"exclude_tags":["hidden"]},"enabled":true}`,
		"--output", "json", "subscription", "channel", "create", "--file", "-",
	)
	var channel application.SubscriptionChannel
	decodeSubscriptionCLIOutput(t, channelOutput, &channel)

	configuration, err := settings.Load(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	database, err := store.Open(context.Background(), filepath.Join(configuration.DataDir, "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	core, err := database.UpsertCoreArtifact(context.Background(), store.CoreArtifact{
		ID: "core-subscription-cli", ExactVersion: "1.13.19",
		OperatingSystem: "linux", Architecture: "arm64", Variant: "plain",
		SourceKind: store.CoreArtifactSourceOfficial, RepositoryID: 1, ReleaseID: 2, AssetID: 3,
		ArchiveSHA256: strings.Repeat("a", 64), BinarySHA256: strings.Repeat("b", 64),
		BinaryPath: filepath.Join(configuration.DataDir, "sing-box"), ReportedVersion: "1.13.19",
		FeatureFingerprint: json.RawMessage(`{"status":"reported","features":["badlinkname","tfogo_checklinkname0","with_acme","with_ccm","with_clash_api","with_dhcp","with_gvisor","with_ocm","with_quic","with_tailscale","with_utls","with_wireguard"]}`),
		VerificationState:  store.CoreArtifactVerified,
		CreatedAt:          now,
	})
	if err != nil {
		_ = database.Close()
		t.Fatal(err)
	}
	startupBytes := []byte(`{
	  "inbounds":[
	    {"type":"shadowsocks","tag":"hidden","listen_port":443,"method":"aes-128-gcm","password":"hidden-password"},
	    {"type":"shadowsocks","tag":"public","listen_port":8443,"method":"aes-256-gcm","password":"public-password"}
	  ]
	}`)
	startup, err := database.CreateStartupArtifact(context.Background(), store.StartupArtifact{
		ID:                  "startup-subscription-cli",
		CanonicalRevisionID: canonicalSave.Revision.ID, ExactCoreVersion: core.ExactVersion,
		AdapterID: "sing-box/v1_13_19/official-linux-plain", AdapterRevision: "2",
		CoreArtifactID: core.ID, ConfigBytes: startupBytes,
		Diagnostics: json.RawMessage(`[]`), CreatedAt: now.Add(time.Second),
	})
	if err == nil {
		startup, err = database.CompleteStartupArtifactCheck(
			context.Background(), startup.ID, true, json.RawMessage(`[]`), now.Add(2*time.Second),
		)
	}
	var user application.SubscriptionUser
	if err == nil {
		instance := application.FromStore(database)
		var prepared application.ActivationPreparation
		prepared, err = instance.PrepareActivationBundle(context.Background(), startup.ID, store.MonitoringProcessOnly)
		if err == nil {
			var task application.Task
			task, err = instance.QueueRuntimeApply(context.Background(), prepared.Bundle.ID)
			if err == nil {
				var claimed *store.Task
				claimed, err = database.ClaimTask(context.Background(), store.ClaimTaskInput{
					Lane: store.TaskLaneRuntime, LeaseOwner: "subscription-cli-test",
					Now: time.Now().UTC(), LeaseDuration: time.Minute,
				})
				if err == nil && (claimed == nil || claimed.ID != task.ID) {
					err = errors.New("runtime apply task was not claimable")
				}
				if err == nil {
					_, err = database.CompleteTask(context.Background(), claimed.ID, claimed.LeaseOwner,
						time.Now().UTC(), store.TaskCompletion{Succeeded: true})
				}
			}
		}
		if err == nil {
			user, err = instance.CreateSubscriptionUser(context.Background(), application.CreateSubscriptionUserRequest{
				Name: "CLI preview user", Enabled: true,
			})
		}
		if err == nil {
			var catalog application.SubscriptionNodeCatalog
			catalog, err = instance.SubscriptionNodeCatalog(context.Background())
			if err == nil {
				var publicKey string
				for _, node := range catalog.Nodes {
					if node.Tag == "public" {
						publicKey = node.Key
					}
				}
				if publicKey == "" {
					err = errors.New("public node was not found")
				} else {
					_, err = instance.ReplaceSubscriptionUserGrants(context.Background(), user.ID, []string{publicKey}, user.UpdatedAt)
				}
			}
		}
	}
	if closeErr := database.Close(); err == nil && closeErr != nil {
		err = closeErr
	}
	if err != nil {
		t.Fatal(err)
	}

	previewOutput := runApplicationCommand(t, settingsPath, "",
		"--output", "json", "subscription", "channel", "render", channel.ID, "--user", user.ID,
	)
	var preview application.SubscriptionPreview
	decodeSubscriptionCLIOutput(t, previewOutput, &preview)
	if preview.ExactCoreVersion != "1.13.19" || preview.Result.NodeCount != 1 ||
		!bytes.Contains(preview.Result.Content, []byte(`"tag":"public"`)) ||
		bytes.Contains(preview.Result.Content, []byte(`"tag":"hidden"`)) {
		t.Fatalf("preview = %+v, content=%s", preview, preview.Result.Content)
	}

	textOutput := runApplicationCommand(t, settingsPath, "",
		"--output", "text", "subscription", "channel", "render", channel.ID, "--user", user.ID,
	)
	if !bytes.Contains(textOutput, []byte(`"tag":"public"`)) || bytes.Contains(textOutput, []byte(`"tag":"hidden"`)) {
		t.Fatalf("text preview = %s", textOutput)
	}
}

func TestSubscriptionTokenCLIPlaintextLifecycle(t *testing.T) {
	settingsPath := commandSettingsFixture(t)
	instance, err := application.Open(context.Background(), settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	user, err := instance.CreateSubscriptionUser(context.Background(), application.CreateSubscriptionUserRequest{
		Name: "CLI token user", Enabled: true,
	})
	if closeErr := instance.Close(); err == nil && closeErr != nil {
		err = closeErr
	}
	if err != nil {
		t.Fatal(err)
	}
	expiresAt := time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339Nano)
	createdOutput := runApplicationCommand(t, settingsPath, "",
		"--output", "json", "subscription", "token", "create",
		"--user-id", user.ID, "--label", "primary", "--expires-at", expiresAt,
	)
	var created application.CreatedSubscriptionToken
	decodeSubscriptionCLIOutput(t, createdOutput, &created)
	if created.Token == "" || !created.Metadata.Active || bytes.Count(createdOutput, []byte(created.Token)) != 1 {
		t.Fatalf("created token = %+v; output=%s", created, createdOutput)
	}

	listedOutput := runApplicationCommand(t, settingsPath, "",
		"--output", "json", "subscription", "token", "list",
	)
	assertSubscriptionTokensDoNotLeak(t, listedOutput, created.Token)
	var listed application.SubscriptionTokenPage
	decodeSubscriptionCLIOutput(t, listedOutput, &listed)
	if len(listed.Items) != 1 || listed.Items[0].ID != created.Metadata.ID {
		t.Fatalf("listed tokens = %+v", listed)
	}

	rotationOutput := runApplicationCommand(t, settingsPath, "",
		"--output", "json", "subscription", "token", "rotate", created.Metadata.ID,
	)
	var rotation application.SubscriptionTokenRotation
	decodeSubscriptionCLIOutput(t, rotationOutput, &rotation)
	if rotation.Token == "" || rotation.Token == created.Token || rotation.Revoked.Active || !rotation.Created.Active ||
		bytes.Count(rotationOutput, []byte(rotation.Token)) != 1 || bytes.Contains(rotationOutput, []byte(created.Token)) {
		t.Fatalf("rotation = %+v; output=%s", rotation, rotationOutput)
	}

	listedJSONL := runApplicationCommand(t, settingsPath, "",
		"--output", "jsonl", "subscription", "token", "list",
	)
	assertSubscriptionTokensDoNotLeak(t, listedJSONL, created.Token, rotation.Token)
	if bytes.Count(listedJSONL, []byte{'\n'}) != 2 {
		t.Fatalf("token JSONL = %s", listedJSONL)
	}

	revokedOutput := runApplicationCommand(t, settingsPath, "",
		"--output", "json", "subscription", "token", "revoke", rotation.Created.ID,
	)
	var revoked application.SubscriptionToken
	decodeSubscriptionCLIOutput(t, revokedOutput, &revoked)
	if revoked.Active || revoked.RevokedAt == nil || bytes.Contains(revokedOutput, []byte(rotation.Token)) {
		t.Fatalf("revoked = %+v; output=%s", revoked, revokedOutput)
	}
}

func TestSubscriptionCLIRequiresFileCASAndDoesNotEchoSensitiveInput(t *testing.T) {
	settingsPath := commandSettingsFixture(t)
	secret := "sensitive-subscription-credential-DO-NOT-ECHO"

	stdout, stderr, err := executeSubscriptionCLI(
		t, settingsPath, secret,
		"subscription", "channel", "create",
	)
	assertSubscriptionCLIError(t, err, ErrorUsage, "subscription_file_required")
	if strings.Contains(stdout+stderr+err.Error(), secret) {
		t.Fatalf("missing-file error leaked stdin secret: stdout=%q stderr=%q err=%v", stdout, stderr, err)
	}

	stdout, stderr, err = executeSubscriptionCLI(
		t, settingsPath,
		`{"name":"safe","format":"sing-box","public_host":"safe.example","config":{},"enabled":true,"unexpected":"`+secret+`"}`,
		"subscription", "channel", "create", "--file", "-",
	)
	assertSubscriptionCLIError(t, err, ErrorValidation, "subscription_input_invalid")
	if strings.Contains(stdout+stderr+err.Error(), secret) {
		t.Fatalf("strict-input error leaked file content: stdout=%q stderr=%q err=%v", stdout, stderr, err)
	}

	stdout, stderr, err = executeSubscriptionCLI(
		t, settingsPath, secret,
		"subscription", "source", "refresh", "source_missing",
	)
	assertSubscriptionCLIError(t, err, ErrorDomain, "subscription_source_not_found")
	if strings.Contains(stdout+stderr+err.Error(), secret) {
		t.Fatalf("CAS error leaked secret: stdout=%q stderr=%q err=%v", stdout, stderr, err)
	}

	root := NewRootCommand(Dependencies{OpenApplication: application.Open})
	channelCreate, _, findErr := root.Find([]string{"subscription", "channel", "create"})
	if findErr != nil {
		t.Fatal(findErr)
	}
	if channelCreate.Flags().Lookup("file") == nil {
		t.Fatal("channel create has no --file input")
	}
	for _, forbidden := range []string{"body", "config-json", "payload", "secret", "token"} {
		if channelCreate.LocalNonPersistentFlags().Lookup(forbidden) != nil {
			t.Errorf("channel create unexpectedly accepts --%s in argv", forbidden)
		}
	}
	tokenCreate, _, findErr := root.Find([]string{"subscription", "token", "create"})
	if findErr != nil {
		t.Fatal(findErr)
	}
	for _, forbidden := range []string{"secret", "token"} {
		if tokenCreate.LocalNonPersistentFlags().Lookup(forbidden) != nil {
			t.Errorf("token create unexpectedly accepts --%s in argv", forbidden)
		}
	}
}

func executeSubscriptionCLI(
	t *testing.T,
	settingsPath string,
	stdin string,
	args ...string,
) (string, string, error) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	command := NewRootCommand(Dependencies{
		Stdin: strings.NewReader(stdin), Stdout: &stdout, Stderr: &stderr,
		OpenApplication: application.Open,
	})
	command.SetArgs(append([]string{"--config", settingsPath}, args...))
	err := command.ExecuteContext(context.Background())
	return stdout.String(), stderr.String(), err
}

func decodeSubscriptionCLIOutput(t *testing.T, output []byte, target any) {
	t.Helper()
	if err := json.Unmarshal(output, target); err != nil {
		t.Fatalf("decode subscription CLI output: %v; output=%s", err, output)
	}
}

func assertSubscriptionCLIError(t *testing.T, err error, kind ErrorKind, code string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected %s error", code)
	}
	var classified *Error
	if !errors.As(err, &classified) || classified.Kind != kind || classified.Code != code {
		t.Fatalf("error = %#v, want kind=%v code=%q", err, kind, code)
	}
}

func assertSubscriptionTokensDoNotLeak(t *testing.T, output []byte, secrets ...string) {
	t.Helper()
	for _, secret := range secrets {
		if bytes.Contains(output, []byte(secret)) {
			t.Fatalf("token list leaked plaintext %q: %s", secret, output)
		}
	}
	for _, forbidden := range [][]byte{[]byte(`"token":`), []byte("token_sha256")} {
		if bytes.Contains(output, forbidden) {
			t.Fatalf("token list leaked forbidden field %q: %s", forbidden, output)
		}
	}
}
