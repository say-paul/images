// Package manifest is used to define an osbuild manifest as a series of
// pipelines with content. Typically, a Manifest is created using
// manifest.New() and pipelines are defined and added to it using the pipeline
// constructors (e.g., NewBuild()) with the manifest as the first argument. The
// pipelines are added in the order they are called.
//
// The package implements a standard set of osbuild pipelines. A pipeline
// conceptually represents a named filesystem tree, optionally generated
// in a provided build root (represented by another pipeline). All inputs
// to a pipeline must be explicitly specified, either in terms of another
// pipeline, in terms of content addressable inputs or in terms of static
// parameters to the inherited Pipeline structs.
package manifest

import (
	"encoding/json"
	"fmt"

	"github.com/osbuild/images/internal/common"
	"github.com/osbuild/images/pkg/container"
	"github.com/osbuild/images/pkg/dnfjson"
	"github.com/osbuild/images/pkg/osbuild"
	"github.com/osbuild/images/pkg/ostree"
	"github.com/osbuild/images/pkg/rpmmd"
)

type Distro uint64

const (
	DISTRO_NULL = iota
	DISTRO_EL10
	DISTRO_EL9
	DISTRO_EL8
	DISTRO_EL7
	DISTRO_FEDORA
)

func (d *Distro) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}

	switch s {
	case "rhel-10":
		*d = DISTRO_EL10
	case "rhel-9":
		*d = DISTRO_EL9
	case "rhel-8":
		*d = DISTRO_EL8
	case "rhel-7":
		*d = DISTRO_EL7
	case "fedora":
		*d = DISTRO_FEDORA
	default:
		return fmt.Errorf("unknown distro: %q", s)
	}
	return nil
}

func (d *Distro) UnmarshalYAML(unmarshal func(any) error) error {
	return common.UnmarshalYAMLviaJSON(d, unmarshal)
}

type Inputs osbuild.SourceInputs

// An OSBuildManifest is an opaque JSON object, which is a valid input to osbuild
type OSBuildManifest []byte

func (m OSBuildManifest) MarshalJSON() ([]byte, error) {
	return json.RawMessage(m).MarshalJSON()
}

func (m *OSBuildManifest) UnmarshalJSON(payload []byte) error {
	var raw json.RawMessage
	err := (&raw).UnmarshalJSON(payload)
	if err != nil {
		return err
	}
	*m = OSBuildManifest(raw)
	return nil
}

// Manifest represents a manifest initialised with all the information required
// to generate the pipelines but no content. The content type sources
// (PackageSetChains, ContainerSourceSpecs, OSTreeSourceSpecs) must be
// retrieved through their corresponding Getters and resolved before
// serializing.
type Manifest struct {

	// pipelines describe the build process for an image.
	pipelines []Pipeline

	// Distro defines the distribution of the image that this manifest will
	// generate. It is used for determining package names that differ between
	// different distributions and version.
	Distro Distro

	// DistroBootstrapRef defines if a bootstrap container should be used
	// to generate the buildroot
	// XXX: ideally we would have "Distro distro.Distro" here and a
	// "BoostrapContainerRef()" method on this but we cannot because of
	// circular imports so we use the same workaround as Distro above.
	DistroBootstrapRef string
}

func New() Manifest {
	return Manifest{
		pipelines: make([]Pipeline, 0),
		Distro:    DISTRO_NULL,
	}
}

func (m *Manifest) addPipeline(p Pipeline) {
	for _, pipeline := range m.pipelines {
		if pipeline.Name() == p.Name() {
			panic(fmt.Errorf("duplicate pipeline name %v in manifest", p.Name()))
		}
	}
	if p.Manifest() != nil {
		panic(fmt.Errorf("pipeline %v already added to a different manifest", p.Name()))
	}
	m.pipelines = append(m.pipelines, p)
	p.setManifest(m)
	// check that the pipeline's build pipeline is included in the same manifest
	if build := p.BuildPipeline(); build != nil && build.Manifest() != m {
		panic("cannot add pipeline to a different manifest than its build pipeline")
	}
}

type PackageSelector func([]rpmmd.PackageSet) []rpmmd.PackageSet

func (m Manifest) GetPackageSetChains() map[string][]rpmmd.PackageSet {
	chains := make(map[string][]rpmmd.PackageSet)

	for _, pipeline := range m.pipelines {
		if chain := pipeline.getPackageSetChain(m.Distro); chain != nil {
			chains[pipeline.Name()] = chain
		}
	}

	return chains
}

func (m Manifest) GetContainerSourceSpecs() map[string][]container.SourceSpec {
	// Containers should only appear in the payload pipeline.
	// Let's iterate over all pipelines to avoid assuming pipeline names, but
	// return all the specs as a single slice.
	containerSpecs := make(map[string][]container.SourceSpec)
	for _, pipeline := range m.pipelines {
		if containers := pipeline.getContainerSources(); len(containers) > 0 {
			containerSpecs[pipeline.Name()] = containers
		}
	}
	return containerSpecs
}

func (m Manifest) GetOSTreeSourceSpecs() map[string][]ostree.SourceSpec {
	// OSTree commits should only appear in one pipeline.
	// Let's iterate over all pipelines to avoid assuming pipeline names, but
	// return all the specs as a single slice if there are multiple.
	ostreeSpecs := make(map[string][]ostree.SourceSpec)
	for _, pipeline := range m.pipelines {
		if commits := pipeline.getOSTreeCommitSources(); len(commits) > 0 {
			ostreeSpecs[pipeline.Name()] = commits
		}
	}
	return ostreeSpecs
}

type SerializeOptions struct {
	RpmDownloader osbuild.RpmDownloader
}

func (m Manifest) Serialize(depsolvedSets map[string]dnfjson.DepsolveResult, containerSpecs map[string][]container.Spec, ostreeCommits map[string][]ostree.CommitSpec, opts *SerializeOptions) (OSBuildManifest, error) {
	if opts == nil {
		opts = &SerializeOptions{}
	}

	for _, pipeline := range m.pipelines {
		pipeline.serializeStart(Inputs{
			Depsolved:  depsolvedSets[pipeline.Name()],
			Containers: containerSpecs[pipeline.Name()],
			Commits:    ostreeCommits[pipeline.Name()],
		})
	}

	var pipelines []osbuild.Pipeline
	var mergedInputs osbuild.SourceInputs
	for _, pipeline := range m.pipelines {
		pipelines = append(pipelines, pipeline.serialize())
		mergedInputs.Commits = append(mergedInputs.Commits, pipeline.getOSTreeCommits()...)
		mergedInputs.Depsolved.Packages = append(mergedInputs.Depsolved.Packages, depsolvedSets[pipeline.Name()].Packages...)
		mergedInputs.Depsolved.Repos = append(mergedInputs.Depsolved.Repos, depsolvedSets[pipeline.Name()].Repos...)
		mergedInputs.Containers = append(mergedInputs.Containers, pipeline.getContainerSpecs()...)
		mergedInputs.InlineData = append(mergedInputs.InlineData, pipeline.getInline()...)
		mergedInputs.FileRefs = append(mergedInputs.FileRefs, pipeline.fileRefs()...)
	}
	for _, pipeline := range m.pipelines {
		pipeline.serializeEnd()
	}

	sources, err := osbuild.GenSources(mergedInputs, opts.RpmDownloader)
	if err != nil {
		return nil, err
	}

	return json.Marshal(
		osbuild.Manifest{
			Version:   "2",
			Pipelines: pipelines,
			Sources:   sources,
		},
	)
}

func (m Manifest) GetCheckpoints() []string {
	checkpoints := []string{}
	for _, p := range m.pipelines {
		if p.getCheckpoint() {
			checkpoints = append(checkpoints, p.Name())
		}
	}
	return checkpoints
}

func (m Manifest) GetExports() []string {
	exports := []string{}
	for _, p := range m.pipelines {
		if p.getExport() {
			exports = append(exports, p.Name())
		}
	}
	return exports
}

// filterRepos returns a list of repositories that specify the given pipeline
// name in their PackageSets list in addition to any global repositories
// (global repositories are ones that do not specify any PackageSets).
func filterRepos(repos []rpmmd.RepoConfig, plName string) []rpmmd.RepoConfig {
	filtered := make([]rpmmd.RepoConfig, 0, len(repos))
	for _, repo := range repos {
		if len(repo.PackageSets) == 0 {
			filtered = append(filtered, repo)
			continue
		}
		for _, ps := range repo.PackageSets {
			if ps == plName {
				filtered = append(filtered, repo)
				continue
			}
		}
	}
	return filtered
}
