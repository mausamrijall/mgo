package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mgo-framework/mgo/cli/internal/scaffold"
	"github.com/spf13/cobra"
)

func swapCmd() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "swap <axis> <value>",
		Short: "Switch a stack axis (router, db) in an existing project",
		Long: `Regenerates exactly the files the axis owns and updates go.mod.

Safety: generated files are hash-tracked in mgo.json. If you have edited
one of the files an axis owns, swap refuses to touch it and tells you
what to change instead — --force overrides.`,
		Example: `  mgo swap router stdmux
  mgo swap db sql
  mgo swap db none      # drop the database (or: mgo remove db)`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return swap(args[0], args[1], force)
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "overwrite files even if you edited them")
	return cmd
}

func addCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "add <axis> <value>",
		Short:   "Add a capability to the project (alias of swap)",
		Example: `  mgo add db gorm`,
		Args:    cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return swap(args[0], args[1], false)
		},
	}
	return cmd
}

func removeCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "remove <axis>",
		Short:   "Remove a capability (db → none)",
		Example: `  mgo remove db`,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if args[0] != "db" {
				return fmt.Errorf("only the db axis can be removed")
			}
			return swap("db", "none", false)
		},
	}
}

func swap(axis, value string, force bool) error {
	dir, err := os.Getwd()
	if err != nil {
		return err
	}
	man, err := scaffold.Load(dir)
	if err != nil {
		return err
	}

	allowed, ok := scaffold.Axes[axis]
	if !ok {
		return fmt.Errorf("unknown axis %q (want: router, db)", axis)
	}
	if !contains(allowed, value) {
		return fmt.Errorf("invalid %s %q (want one of: %s)", axis, value, strings.Join(allowed, ", "))
	}

	current := map[string]string{"router": man.Router, "db": man.DB}[axis]
	if current == value {
		step("%s is already %s — nothing to do", axis, value)
		return nil
	}

	// Never overwrite the user's work.
	modified, err := man.Modified(dir, scaffold.Owned[axis]...)
	if err != nil {
		return err
	}
	if len(modified) > 0 && !force {
		fail("you have edited: %s", strings.Join(modified, ", "))
		fmt.Fprintf(os.Stderr, "\nThese files belong to the %s axis, so swapping would overwrite your\nchanges. Either apply the swap manually, or rerun with --force.\n", axis)
		return fmt.Errorf("swap aborted to protect your edits")
	}

	opts := man.Options()
	switch axis {
	case "router":
		opts.Router = value
	case "db":
		opts.DB = value
	}

	files, err := scaffold.RenderOwned(opts, axis)
	if err != nil {
		return err
	}
	if err := scaffold.WriteFiles(dir, files, man); err != nil {
		return err
	}
	// db → none drops store.go.
	if axis == "db" && value == "none" {
		if err := os.Remove(filepath.Join(dir, "store.go")); err != nil && !os.IsNotExist(err) {
			return err
		}
		delete(man.Generated, "store.go")
	}
	step("regenerated %s", strings.Join(sortedKeys(files), ", "))

	// go.mod: ensure replace lines for the new stack, then tidy. Uses
	// `go mod edit`, so user-added requires and replaces survive.
	for _, r := range opts.Replaces() {
		if err := run(dir, "go", "mod", "edit", "-replace", r.Mod+"="+r.Path); err != nil {
			return fmt.Errorf("go mod edit: %w", err)
		}
	}
	if err := run(dir, "go", "mod", "tidy"); err != nil {
		return fmt.Errorf("go mod tidy: %w", err)
	}
	step("go.mod updated")

	man.Router, man.DB = opts.Router, opts.DB
	if err := man.Save(dir); err != nil {
		return err
	}

	if err := run(dir, "go", "test", "./..."); err != nil {
		return fmt.Errorf("tests failed after swap: %w", err)
	}
	step("tests passing on %s=%s", axis, value)
	return nil
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	for i := range out {
		for j := i + 1; j < len(out); j++ {
			if out[j] < out[i] {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}
