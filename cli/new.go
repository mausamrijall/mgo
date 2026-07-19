package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/mgo-framework/mgo/cli/internal/scaffold"
	"github.com/spf13/cobra"
)

// presets are opinionated axis bundles — the fastest golden path.
var presets = map[string]struct{ router, db string }{
	"minimal": {"stdmux", "none"},
	"api":     {"chi", "gorm"},
}

func newCmd() *cobra.Command {
	var router, db, preset, module string
	var skipChecks bool

	cmd := &cobra.Command{
		Use:   "new [name]",
		Short: "Create a new MGO application (compiling and tested)",
		Long: `Creates a new MGO application: native router code, plain net/http
handlers, passing tests, and an mgo.json manifest for later swaps.

With no flags, an interactive wizard asks the two questions that matter.
Presets and flags skip the questions entirely.`,
		Example: `  mgo new blog
  mgo new api --preset api
  mgo new svc --router chi --db sql`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			start := time.Now()

			name := ""
			if len(args) == 1 {
				name = args[0]
			}
			if p, ok := presets[preset]; ok {
				if router == "" {
					router = p.router
				}
				if db == "" {
					db = p.db
				}
			} else if preset != "" {
				return fmt.Errorf("unknown preset %q (want: minimal, api)", preset)
			}

			// Wizard fills whatever flags left open (TTY only).
			interactive := isTTY && (name == "" || router == "" || db == "")
			if name == "" {
				if !interactive {
					return fmt.Errorf("project name required (mgo new <name>)")
				}
				name = ask("Project name", "")
				if name == "" {
					return fmt.Errorf("project name required")
				}
			}
			if router == "" {
				if interactive {
					router = choose("Router — routes are written in its native API", scaffold.Axes["router"], "chi")
				} else {
					router = "chi"
				}
			}
			if db == "" {
				if interactive {
					db = choose("Database", scaffold.Axes["db"], "none")
				} else {
					db = "none"
				}
			}
			if module == "" {
				module = name
			}

			mgoSrc, err := findMGOSrc()
			if err != nil {
				return err
			}

			opts := scaffold.Options{Name: name, Module: module, Router: router, DB: db, MGOSrc: mgoSrc}
			files, err := scaffold.Render(opts)
			if err != nil {
				return err
			}

			dir := name
			if _, err := os.Stat(dir); err == nil {
				return fmt.Errorf("directory %s already exists", dir)
			}
			man := &scaffold.Manifest{Name: name, Module: module, Router: router, DB: db, MGOSrc: mgoSrc, Generated: map[string]string{}}
			if err := scaffold.WriteFiles(dir, files, man); err != nil {
				return err
			}
			if err := man.Save(dir); err != nil {
				return err
			}
			step("%d files created in ./%s  %s", len(files)+1, dir, dim("(router: "+router+", db: "+db+")"))

			if err := run(dir, "go", "mod", "tidy"); err != nil {
				return fmt.Errorf("go mod tidy: %w", err)
			}
			step("dependencies resolved")

			if !skipChecks {
				if err := run(dir, "go", "test", "./..."); err != nil {
					return fmt.Errorf("generated tests failed (this is a bug in mgo): %w", err)
				}
				step("tests passing")
			}

			fmt.Printf("\n%s in %s\n\n", bold("Ready"), time.Since(start).Round(time.Millisecond))
			fmt.Printf("  cd %s\n", name)
			fmt.Printf("  mgo dev        %s\n", dim("# run with hot reload"))
			fmt.Printf("  go test ./...  %s\n\n", dim("# your app came with tests"))
			return nil
		},
	}

	cmd.Flags().StringVar(&router, "router", "", "router: "+strings.Join(scaffold.Axes["router"], " | "))
	cmd.Flags().StringVar(&db, "db", "", "database: "+strings.Join(scaffold.Axes["db"], " | "))
	cmd.Flags().StringVar(&preset, "preset", "", "preset: minimal | api")
	cmd.Flags().StringVar(&module, "module", "", "go module path (default: project name)")
	cmd.Flags().BoolVar(&skipChecks, "skip-checks", false, "skip running the generated tests")
	return cmd
}

// findMGOSrc locates the MGO source tree for replace directives: $MGO_SRC
// first, then walking up from the working directory.
func findMGOSrc() (string, error) {
	if src := os.Getenv("MGO_SRC"); src != "" {
		if !looksLikeMGO(src) {
			return "", fmt.Errorf("MGO_SRC=%s does not look like an MGO source tree", src)
		}
		return filepath.Abs(src)
	}
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if looksLikeMGO(dir) {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("cannot find the MGO source tree — set MGO_SRC or run inside a checkout of github.com/mausamrijall/mgo")
		}
		dir = parent
	}
}

func looksLikeMGO(dir string) bool {
	raw, err := os.ReadFile(filepath.Join(dir, "contracts", "go.mod"))
	return err == nil && strings.Contains(string(raw), "module github.com/mgo-framework/mgo/contracts")
}

// run executes a command in dir, streaming output only on failure.
func run(dir string, name string, args ...string) error {
	c := exec.Command(name, args...)
	c.Dir = dir
	out, err := c.CombinedOutput()
	if err != nil {
		os.Stderr.Write(out)
	}
	return err
}
