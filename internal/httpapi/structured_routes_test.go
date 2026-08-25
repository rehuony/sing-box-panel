// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/capability"
	"github.com/rehuony/sing-box-panel/internal/coreartifact"
	"github.com/rehuony/sing-box-panel/internal/runtimeidentity"
)

func TestStructuredRenderAndManualReattachHTTPUseImmutableEvidence(t *testing.T) {
	handler, database := newCoreHTTPFixture(t)
	core := seedCoreHTTPArtifact(t, database)
	handler.commands = application.FromStoreWithRuntimeResolver(database, structuredHTTPRuntimeResolver{
		identity: runtimeidentity.Identity{
			PID: 42, ExactCoreVersion: core.ExactVersion, CoreArtifactID: core.ID,
			ArchiveSHA256: core.ArchiveSHA256, BinarySHA256: core.BinarySHA256,
		},
	})
	initial, err := handler.commands.ReplaceCanonical(context.Background(), "", []byte(`{"schema_version":1,"global":{"mode":"direct"},"nodes":[],"rules":[],"subscription":{}}`))
	if err != nil {
		t.Fatal(err)
	}
	generation, commit, digest := structuredHTTPGeneration(t, "1.13.19")
	if _, err := handler.commands.RefreshCapabilityGeneration(context.Background(), generation); err != nil {
		t.Fatal(err)
	}
	if _, err := handler.commands.UpgradeCapability(context.Background(), application.CapabilityUpgradeRequest{
		ExactCoreVersion: "1.13.19", CommitSHA: commit, ManifestSHA256: digest,
	}); err != nil {
		t.Fatal(err)
	}

	render := authenticatedRequest(handler, http.MethodPost, "/api/v1/config/render",
		`{"allow_compatible":false}`, "")
	if render.Code != http.StatusAccepted {
		t.Fatalf("render status=%d body=%s", render.Code, render.Body.String())
	}
	var rendered application.StructuredRender
	if err := json.Unmarshal(render.Body.Bytes(), &rendered); err != nil {
		t.Fatal(err)
	}
	if rendered.Resolution.Source != "running" || rendered.Resolution.ExactVersion != core.ExactVersion ||
		rendered.Resolution.Running == nil || rendered.Resolution.Running.CoreArtifactID != core.ID ||
		rendered.Artifact.CoreArtifactID != core.ID || rendered.Artifact.CapabilityDigest != digest ||
		string(rendered.Artifact.Config) != `{"route":{"final":"direct"}}` || rendered.Task.StartupArtifactID != rendered.Artifact.ID {
		t.Fatalf("rendered = %+v", rendered)
	}
	listedResponse := authenticatedRequest(handler, http.MethodGet,
		"/api/v1/config/artifacts?core_version=1.13.19&kind=structured&limit=10", "", "")
	if listedResponse.Code != http.StatusOK || strings.Contains(listedResponse.Body.String(), `"config"`) ||
		strings.Contains(listedResponse.Body.String(), `"final"`) {
		t.Fatalf("startup list status=%d body=%s", listedResponse.Code, listedResponse.Body.String())
	}
	var listed application.StartupArtifactPage
	if err := json.Unmarshal(listedResponse.Body.Bytes(), &listed); err != nil ||
		len(listed.Items) != 1 || listed.Items[0].ID != rendered.Artifact.ID {
		t.Fatalf("startup list = %+v, %v", listed, err)
	}

	detached, err := handler.commands.DetachManualJSON(context.Background(), rendered.Artifact.ID)
	if err != nil {
		t.Fatal(err)
	}
	current, err := handler.commands.SetCanonicalValue(context.Background(), initial.Revision.ID, "/global/mode", json.RawMessage(`"block"`))
	if err != nil {
		t.Fatal(err)
	}
	previewResponse := authenticatedRequest(handler, http.MethodGet,
		"/api/v1/config/manual/"+detached.Artifact.ID+"/reattach/preview", "", "")
	if previewResponse.Code != http.StatusOK {
		t.Fatalf("preview status=%d body=%s", previewResponse.Code, previewResponse.Body.String())
	}
	var preview application.ManualReattachPreview
	if err := json.Unmarshal(previewResponse.Body.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	if preview.Evidence.CurrentHeadID != current.Revision.ID || len(preview.Conflicts) != 0 {
		t.Fatalf("preview = %+v", preview)
	}
	body, err := json.Marshal(map[string]any{"evidence": preview.Evidence, "decisions": map[string]string{}})
	if err != nil {
		t.Fatal(err)
	}
	apply := authenticatedRequest(handler, http.MethodPost,
		"/api/v1/config/manual/"+detached.Artifact.ID+"/reattach", string(body), "")
	if apply.Code != http.StatusAccepted || apply.Header().Get("ETag") == "" {
		t.Fatalf("reattach status=%d etag=%q body=%s", apply.Code, apply.Header().Get("ETag"), apply.Body.String())
	}
	var saved application.ManualReattachSave
	if err := json.Unmarshal(apply.Body.Bytes(), &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Artifact.ID == detached.Artifact.ID || saved.Artifact.Raw != detached.Artifact.Raw || saved.Task.Kind != "startup-check" {
		t.Fatalf("reattached = %+v", saved)
	}
}

func TestStructuredHTTPOmissionRequiresRunningIdentityAndRejectsLatest(t *testing.T) {
	handler, _ := newCoreHTTPFixture(t)
	missingVersion := authenticatedRequest(handler, http.MethodPost, "/api/v1/config/render", `{}`, "")
	assertCoreHTTPProblem(t, missingVersion, http.StatusConflict, "core_not_running")
	latest := authenticatedRequest(handler, http.MethodPost, "/api/v1/config/render", `{"core_version":"latest"}`, "")
	assertCoreHTTPProblem(t, latest, http.StatusUnprocessableEntity, "core_version_invalid")
	unknown := authenticatedRequest(handler, http.MethodPost, "/api/v1/config/render", `{"core_version":"1.13.19","latest":true}`, "")
	assertCoreHTTPProblem(t, unknown, http.StatusUnprocessableEntity, "invalid_json")
	invalidReattach := authenticatedRequest(handler, http.MethodPost,
		"/api/v1/config/manual/startup/reattach", `{"evidence":{},"decisions":{},"raw":"secret"}`, "")
	assertCoreHTTPProblem(t, invalidReattach, http.StatusUnprocessableEntity, "invalid_json")
}

type structuredHTTPRuntimeResolver struct {
	identity runtimeidentity.Identity
	err      error
}

func (resolver structuredHTTPRuntimeResolver) Resolve(context.Context) (runtimeidentity.Identity, error) {
	return resolver.identity, resolver.err
}

func structuredHTTPGeneration(t *testing.T, exactVersion string) ([]byte, string, string) {
	t.Helper()
	version, err := coreartifact.ParseExactVersion(exactVersion)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := capability.NewManifest(capability.ManifestSpec{
		SchemaVersion: capability.ManifestSchemaVersion,
		CoreVersion:   version,
		SupportLevel:  capability.SupportNativeStructured,
		SemanticFacts: []capability.SemanticFact{{
			ID: "global.mode", CanonicalPath: "/global/mode",
			Classification: capability.CoverageSupported, OwnedPaths: []string{"/route/final"},
		}},
		Transforms: []capability.Transform{{
			ID: "global.mode.rename", FactID: "global.mode", Primitive: capability.PrimitiveRename,
			From: []string{"/global/mode"}, To: []string{"/route/final"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	manifestJSON, err := manifest.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	digest, err := manifest.Digest()
	if err != nil {
		t.Fatal(err)
	}
	commit := strings.Repeat("7", 40)
	envelope := map[string]any{
		"schema_version": capability.GenerationSchemaVersion,
		"repository":     capability.ManifestRepository,
		"commit_sha":     commit,
		"manifest_count": 1,
		"manifests": []map[string]any{{
			"path":            "capabilities/" + exactVersion + ".json",
			"manifest_sha256": digest.String(),
			"manifest":        json.RawMessage(manifestJSON),
		}},
	}
	encoded, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	return encoded, commit, digest.String()
}
