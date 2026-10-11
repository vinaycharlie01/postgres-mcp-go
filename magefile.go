//go:build mage

// Mage build file for postgres-mcp-go.
// Powered by nava (https://github.com/nirantaraai/nava).
//
// Usage:
//
//	go install github.com/magefile/mage@latest
//	mage -l          # list targets
//	mage build       # compile for current platform
//	mage test        # run tests
//	mage lint        # run golangci-lint
package main

import (
	"fmt"
	"os"

	"github.com/magefile/mage/mg"
	dockermagex "github.com/nirantaraai/nava/mage/docker"
	gomagex "github.com/nirantaraai/nava/mage/golang"
	goreleasermagex "github.com/nirantaraai/nava/mage/goreleaser"
)

func init() {
	_ = gomagex.LoadConfig("go.yaml")
	_ = dockermagex.LoadConfig("docker.yaml")
	_ = goreleasermagex.LoadConfig("goreleaser.yaml")
}

// Build compiles postgres-mcp-go for the current platform (config: go.yaml → build).
func Build() error { return gomagex.Build() }

// Test runs the unit test suite (config: go.yaml → test).
func Test() error { return gomagex.Test() }

// Lint runs golangci-lint (config: go.yaml → lint).
func Lint() error { return gomagex.Lint() }

// Vet runs go vet (config: go.yaml → vet).
func Vet() error { return gomagex.Vet() }

// Setup downloads Go modules (config: go.yaml → setup).
func Setup() error { return gomagex.Setup() }

// Race runs tests with race detection (config: go.yaml → race).
func Race() error { return gomagex.Race() }

// Coverage runs tests with coverage profiling (config: go.yaml → coverage).
func Coverage() error { return gomagex.Coverage() }

// Bench runs benchmarks (config: go.yaml → bench).
func Bench() error { return gomagex.Bench() }

// Govulncheck runs govulncheck for vulnerability scanning (config: go.yaml → govulncheck).
func Govulncheck() error { return gomagex.Govulncheck() }

// BuildLinux cross-compiles for linux/amd64 and linux/arm64 (config: go.yaml → crossBuild).
func BuildLinux() error { return gomagex.CrossBuild() }

// Clean removes build artefacts.
func Clean() error {
	fmt.Println("cleaning dist/")
	return os.RemoveAll("dist")
}

// Docker namespace for container operations.
type Docker mg.Namespace

// Build builds a multi-platform container image (config: docker.yaml → buildxBuild).
func (Docker) Build() error { return dockermagex.BuildxBuild() }

// Push pushes the image to the registry (config: docker.yaml → push).
func (Docker) Push() error { return dockermagex.Push() }

// Login logs in to the container registry (config: docker.yaml → login).
func (Docker) Login() error { return dockermagex.Login() }

// Release creates a GitHub release via goreleaser (config: goreleaser.yaml).
func Release() error { return goreleasermagex.Release() }

// Snapshot creates a local snapshot build without publishing (config: goreleaser.yaml).
func Snapshot() error { return goreleasermagex.Snapshot() }
