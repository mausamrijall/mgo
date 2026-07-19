package scaffold

// Snapshot tests over the full stack matrix: every generated file for
// every (router × db) combination is pinned as a golden file. Template
// changes must be deliberate: go test ./internal/scaffold -update

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite golden files")

func TestRenderMatrixSnapshots(t *testing.T) {
	for _, router := range Axes["router"] {
		for _, db := range Axes["db"] {
			combo := router + "-" + db
			t.Run(combo, func(t *testing.T) {
				files, err := Render(Options{
					Name: "demo", Module: "demo",
					Router: router, DB: db, MGOSrc: "/mgo",
				})
				if err != nil {
					t.Fatal(err)
				}
				dir := filepath.Join("testdata", combo)
				if *update {
					os.RemoveAll(dir)
					if err := os.MkdirAll(dir, 0o755); err != nil {
						t.Fatal(err)
					}
					for name, content := range files {
						if err := os.WriteFile(filepath.Join(dir, name+".golden"), []byte(content), 0o644); err != nil {
							t.Fatal(err)
						}
					}
					return
				}
				goldens, err := filepath.Glob(filepath.Join(dir, "*.golden"))
				if err != nil || len(goldens) == 0 {
					t.Fatalf("no golden files for %s (run with -update)", combo)
				}
				if len(goldens) != len(files) {
					t.Fatalf("file count drifted: %d generated, %d goldens (run with -update after review)", len(files), len(goldens))
				}
				for _, g := range goldens {
					name := strings.TrimSuffix(filepath.Base(g), ".golden")
					want, err := os.ReadFile(g)
					if err != nil {
						t.Fatal(err)
					}
					got, ok := files[name]
					if !ok {
						t.Fatalf("golden %s has no generated counterpart", name)
					}
					if got != string(want) {
						t.Fatalf("%s drifted from snapshot (run with -update after review)\n--- got ---\n%s", name, got)
					}
				}
			})
		}
	}
}

func TestOwnedFilesExistInRender(t *testing.T) {
	files, err := Render(Options{Name: "d", Module: "d", Router: "chi", DB: "gorm", MGOSrc: "/m"})
	if err != nil {
		t.Fatal(err)
	}
	for axis, owned := range Owned {
		for _, f := range owned {
			if _, ok := files[f]; !ok {
				t.Fatalf("axis %s owns %s but Render does not produce it", axis, f)
			}
		}
	}
}

func TestManifestModifiedDetection(t *testing.T) {
	dir := t.TempDir()
	m := &Manifest{Generated: map[string]string{}}
	if err := WriteFiles(dir, map[string]string{"a.go": "package main\n"}, m); err != nil {
		t.Fatal(err)
	}
	mod, err := m.Modified(dir)
	if err != nil || len(mod) != 0 {
		t.Fatalf("fresh file reported modified: %v %v", mod, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package main // edited\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mod, err = m.Modified(dir)
	if err != nil || len(mod) != 1 || mod[0] != "a.go" {
		t.Fatalf("edit not detected: %v %v", mod, err)
	}
}
