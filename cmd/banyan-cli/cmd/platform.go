package cmd

import (
	"sort"
	"strings"

	"github.com/fertile-org/banyan/pkg/logging"
	"github.com/fertile-org/banyan/pkg/rpc/banyanpb"
	"github.com/fertile-org/banyan/pkg/types"
)

// clusterArches builds a name→arch map (omitting agents that report no arch)
// and the sorted set of distinct arches present in the cluster.
func clusterArches(agents []*banyanpb.AgentInfo) (map[string]string, []string) {
	archOf := map[string]string{}
	set := map[string]bool{}
	for _, a := range agents {
		if a.Arch == "" {
			continue
		}
		archOf[a.Name] = a.Arch
		set[a.Arch] = true
	}
	var all []string
	for arch := range set {
		all = append(all, arch)
	}
	sort.Strings(all)
	return archOf, all
}

// resolveServicePlatform returns the nerdctl --platform value for one build
// service, applying the precedence rules. Returns "" to mean "build host arch"
// (the caller's fallback), which also covers the unknown/offline cases.
func resolveServicePlatform(svc types.ManifestService, archOf map[string]string, allArches []string) string { //nolint:gocritic // svc is a map-range copy; passing by value keeps call sites simple
	// Rule 1: explicit platform wins.
	if svc.Platform != "" {
		return svc.Platform
	}
	// Rule 2: pinned to a node — use that node's arch (if known).
	if svc.Deploy != nil && svc.Deploy.Placement != nil && svc.Deploy.Placement.Node != "" {
		if arch, ok := archOf[svc.Deploy.Placement.Node]; ok {
			return "linux/" + arch
		}
		return "" // unknown node arch → host fallback
	}
	// Rule 4: no placement constraint — union of all cluster arches.
	// (Rule 3, tag-based union, is folded into "all" for now: without a tag
	// filter we target every known arch, which is always safe.)
	if len(allArches) == 0 {
		return "" // no arch info at all → host fallback
	}
	var platforms []string
	for _, a := range allArches {
		platforms = append(platforms, "linux/"+a)
	}
	return strings.Join(platforms, ",")
}

// applyResolvedPlatforms fills Platform on every build service that does not
// already have one, using cluster arch info. Services left empty fall back to
// the build host arch; we warn so the operator knows why.
func applyResolvedPlatforms(services map[string]types.ManifestService, agents []*banyanpb.AgentInfo) {
	archOf, all := clusterArches(agents)
	for name, svc := range services { //nolint:gocritic // map iteration
		if svc.Build == nil || svc.Platform != "" {
			continue
		}
		resolved := resolveServicePlatform(svc, archOf, all)
		if resolved == "" {
			logging.Warn("Could not determine target arch; building for host arch",
				"service", name,
				"hint", "upgrade agents to report arch, or set platform: on the service")
			continue
		}
		svc.Platform = resolved
		services[name] = svc
	}
}
