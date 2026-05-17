package release

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"gopkg.in/yaml.v3"
)

type goReleaserConfig struct {
	Builds []struct {
		Main   string   `yaml:"main"`
		GOOS   []string `yaml:"goos"`
		GOARCH []string `yaml:"goarch"`
	} `yaml:"builds"`
	Archives []struct {
		Format       string `yaml:"format"`
		NameTemplate string `yaml:"name_template"`
	} `yaml:"archives"`
}

type seaportalBuild = struct {
	Main   string   `yaml:"main"`
	GOOS   []string `yaml:"goos"`
	GOARCH []string `yaml:"goarch"`
}

type seaportalArchive = struct {
	Format       string `yaml:"format"`
	NameTemplate string `yaml:"name_template"`
}

func TestGoReleaserBuildMatrix(t *testing.T) {
	cfg := loadGoReleaserConfig(t)

	build, ok := findSeaportalBuild(cfg)
	if !ok {
		t.Fatal("missing build config for ./cmd/seaportal in .goreleaser.yml")
	}

	wantOS := []string{"darwin", "linux", "windows"}
	wantArch := []string{"amd64", "arm64"}

	gotOS := slices.Clone(build.GOOS)
	gotArch := slices.Clone(build.GOARCH)
	slices.Sort(gotOS)
	slices.Sort(gotArch)

	if !slices.Equal(gotOS, wantOS) {
		t.Fatalf("unexpected goos matrix: got %v want %v", gotOS, wantOS)
	}
	if !slices.Equal(gotArch, wantArch) {
		t.Fatalf("unexpected goarch matrix: got %v want %v", gotArch, wantArch)
	}

	total := len(build.GOOS) * len(build.GOARCH)
	if total != 6 {
		t.Fatalf("unexpected binary count: got %d want 6", total)
	}
}

func TestGoReleaserBinaryNaming(t *testing.T) {
	cfg := loadGoReleaserConfig(t)

	build, ok := findSeaportalBuild(cfg)
	if !ok {
		t.Fatal("missing build config for ./cmd/seaportal in .goreleaser.yml")
	}

	archive, ok := findBinaryArchive(cfg)
	if !ok {
		t.Fatal("missing binary archive config in .goreleaser.yml")
	}

	if archive.NameTemplate != "seaportal-{{ .Os }}-{{ .Arch }}" {
		t.Fatalf("unexpected binary name template: got %q", archive.NameTemplate)
	}

	var got []string
	for _, goos := range build.GOOS {
		for _, goarch := range build.GOARCH {
			name := "seaportal-" + goos + "-" + goarch
			if goos == "windows" {
				name += ".exe"
			}
			got = append(got, name)
		}
	}
	slices.Sort(got)

	want := []string{
		"seaportal-darwin-amd64",
		"seaportal-darwin-arm64",
		"seaportal-linux-amd64",
		"seaportal-linux-arm64",
		"seaportal-windows-amd64.exe",
		"seaportal-windows-arm64.exe",
	}

	if !slices.Equal(got, want) {
		t.Fatalf("unexpected artifact names: got %v want %v", got, want)
	}
}

func loadGoReleaserConfig(t *testing.T) goReleaserConfig {
	t.Helper()

	configPath := filepath.Join("..", "..", ".goreleaser.yml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read %s: %v", configPath, err)
	}

	var cfg goReleaserConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("failed to parse %s: %v", configPath, err)
	}

	return cfg
}

func findSeaportalBuild(cfg goReleaserConfig) (seaportalBuild, bool) {
	for _, build := range cfg.Builds {
		if build.Main == "./cmd/seaportal" {
			return build, true
		}
	}
	return seaportalBuild{}, false
}

func findBinaryArchive(cfg goReleaserConfig) (seaportalArchive, bool) {
	for _, archive := range cfg.Archives {
		if archive.Format == "binary" {
			return archive, true
		}
	}
	return seaportalArchive{}, false
}
