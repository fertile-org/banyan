package cmd

import (
	"sort"
	"strings"
	"testing"

	"github.com/fertile-org/banyan/pkg/rpc/banyanpb"
	"github.com/fertile-org/banyan/pkg/types"
)

func TestResolveServicePlatform(t *testing.T) {
	archOf := map[string]string{
		"node-amd": "amd64",
		"node-arm": "arm64",
	}
	all := []string{"amd64", "arm64"}

	tests := []struct {
		name string
		svc  types.ManifestService
		want string
	}{
		{
			name: "explicit platform wins",
			svc:  types.ManifestService{Build: &types.ManifestBuild{Context: "."}, Platform: "linux/riscv64"},
			want: "linux/riscv64",
		},
		{
			name: "placement node arch",
			svc:  types.ManifestService{Build: &types.ManifestBuild{Context: "."}, Deploy: &types.ManifestDeploy{Placement: &types.ManifestPlacement{Node: "node-arm"}}},
			want: "linux/arm64",
		},
		{
			name: "no constraint -> union of all cluster arches",
			svc:  types.ManifestService{Build: &types.ManifestBuild{Context: "."}},
			want: "linux/amd64,linux/arm64",
		},
		{
			name: "placement to unknown node -> host fallback (empty)",
			svc:  types.ManifestService{Build: &types.ManifestBuild{Context: "."}, Deploy: &types.ManifestDeploy{Placement: &types.ManifestPlacement{Node: "ghost"}}},
			want: "",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveServicePlatform(tc.svc, archOf, all)
			if normalizePlatforms(got) != normalizePlatforms(tc.want) {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// normalizePlatforms sorts a comma-list so order does not matter in assertions.
func normalizePlatforms(p string) string {
	if p == "" {
		return ""
	}
	parts := strings.Split(p, ",")
	sort.Strings(parts)
	return strings.Join(parts, ",")
}

func TestClusterArches(t *testing.T) {
	agents := []*fakeAgent{{name: "a", arch: "amd64"}, {name: "b", arch: "arm64"}, {name: "c", arch: ""}}
	archOf, all := clusterArches(toAgentInfo(agents))
	if archOf["a"] != "amd64" || archOf["b"] != "arm64" {
		t.Errorf("archOf wrong: %v", archOf)
	}
	if _, ok := archOf["c"]; ok {
		t.Error("empty-arch agent should be omitted from archOf")
	}
	if normalizePlatforms(strings.Join(all, ",")) != "amd64,arm64" {
		t.Errorf("all arches wrong: %v", all)
	}
}

type fakeAgent struct{ name, arch string }

func toAgentInfo(fs []*fakeAgent) []*banyanpb.AgentInfo {
	var out []*banyanpb.AgentInfo
	for _, f := range fs {
		out = append(out, &banyanpb.AgentInfo{Name: f.name, Arch: f.arch})
	}
	return out
}
