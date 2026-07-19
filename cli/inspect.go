package main

// Manifest-powered inspection: mgo.json is the project's brain, and these
// commands read it. No state of their own, no side effects (doctor only
// diagnoses; a future `mgo repair` would fix).

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/mgo-framework/mgo/cli/internal/scaffold"
	"github.com/spf13/cobra"
)

func infoCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "info",
		Aliases: []string{"graph"},
		Short:   "Show the project's stack (from mgo.json)",
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := os.Getwd()
			if err != nil {
				return err
			}
			man, err := scaffold.Load(dir)
			if err != nil {
				return err
			}
			status, err := man.Status(dir)
			if err != nil {
				return err
			}
			modified := 0
			for _, s := range status {
				if s.State != scaffold.Unchanged {
					modified++
				}
			}

			fmt.Printf("\n%s\n\n", bold("Application Stack — "+man.Name))
			row := func(k, v string) { fmt.Printf("  %-10s %s\n", dim(k), v) }
			row("Router", man.Router)
			row("Database", man.DB)
			row("Module", man.Module)
			row("MGO", man.MGOSrc)
			fmt.Printf("\n  %d generated files tracked", len(status))
			if modified > 0 {
				fmt.Printf(" · %s", cyan(fmt.Sprintf("%d taken over by you (mgo diff)", modified)))
			}
			fmt.Println()
			return nil
		},
	}
}

func diffCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "diff",
		Short: "Show which generated files you have edited",
		Long: `Compares every tracked generated file against the hash recorded in
mgo.json. Unchanged files are safe for mgo to regenerate on swap;
modified files are yours and will never be overwritten (without --force).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := os.Getwd()
			if err != nil {
				return err
			}
			man, err := scaffold.Load(dir)
			if err != nil {
				return err
			}
			status, err := man.Status(dir)
			if err != nil {
				return err
			}
			fmt.Println()
			for _, s := range status {
				switch s.State {
				case scaffold.Unchanged:
					fmt.Printf("  %s  %-14s %s\n", green("✓"), s.Path, dim("unchanged — safe to regenerate"))
				case scaffold.Modified:
					fmt.Printf("  %s  %-14s %s\n", cyan("Δ"), s.Path, "modified manually — cannot swap automatically")
				case scaffold.Deleted:
					fmt.Printf("  %s  %-14s %s\n", red("−"), s.Path, "deleted — taken over")
				}
			}
			fmt.Println()
			return nil
		},
	}
}

func doctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Diagnose the project and toolchain",
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := os.Getwd()
			if err != nil {
				return err
			}
			healthy := true
			check := func(name string, err error) {
				if err == nil {
					step("%s", name)
				} else {
					healthy = false
					fail("%s — %v", name, err)
				}
			}

			check("go toolchain", func() error {
				out, err := exec.Command("go", "version").Output()
				if err != nil {
					return fmt.Errorf("go not on PATH")
				}
				_ = out
				return nil
			}())

			man, err := scaffold.Load(dir)
			check("mgo.json manifest", err)
			if err != nil {
				return fmt.Errorf("doctor found problems")
			}

			check("MGO source tree reachable", func() error {
				if !looksLikeMGO(man.MGOSrc) {
					return fmt.Errorf("%s is not an MGO checkout (fix mgo_src in mgo.json)", man.MGOSrc)
				}
				return nil
			}())

			check("replace directives resolve", func() error {
				for _, r := range man.Options().Replaces() {
					if _, err := os.Stat(filepath.Join(r.Path, "go.mod")); err != nil {
						return fmt.Errorf("%s → %s missing", r.Mod, r.Path)
					}
				}
				return nil
			}())

			status, err := man.Status(dir)
			check("generated files present", func() error {
				if err != nil {
					return err
				}
				var gone []string
				for _, s := range status {
					if s.State == scaffold.Deleted {
						gone = append(gone, s.Path)
					}
				}
				if len(gone) > 0 {
					return fmt.Errorf("deleted: %s", strings.Join(gone, ", "))
				}
				return nil
			}())

			check("project compiles", func() error {
				c := exec.Command("go", "build", "./...")
				c.Dir = dir
				if out, err := c.CombinedOutput(); err != nil {
					return fmt.Errorf("go build failed:\n%s", out)
				}
				return nil
			}())

			if !healthy {
				return fmt.Errorf("doctor found problems")
			}
			fmt.Printf("\n%s\n", bold("All clear."))
			return nil
		},
	}
}
