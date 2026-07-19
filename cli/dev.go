package main

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/mgo-framework/mgo/cli/internal/scaffold"
	"github.com/spf13/cobra"
)

func devCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "dev",
		Short: "Run the app with hot reload",
		Long: `Builds and runs the app, then watches .go files: on change it
rebuilds and restarts. If a rebuild fails, the previous binary keeps
serving and the compile errors are printed — fix and save.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := os.Getwd()
			if err != nil {
				return err
			}
			if _, err := scaffold.Load(dir); err != nil {
				return err
			}
			return dev(dir)
		},
	}
}

func dev(dir string) error {
	bin := filepath.Join(dir, ".mgo", "app")
	if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
		return err
	}

	build := func() error {
		c := exec.Command("go", "build", "-o", bin, ".")
		c.Dir = dir
		out, err := c.CombinedOutput()
		if err != nil {
			fmt.Fprint(os.Stderr, string(out))
		}
		return err
	}

	var child *exec.Cmd
	start := func() {
		child = exec.Command(bin)
		child.Dir = dir
		child.Stdout, child.Stderr = os.Stdout, os.Stderr
		child.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		if err := child.Start(); err != nil {
			fail("start: %v", err)
			child = nil
		}
	}
	stop := func() {
		if child == nil || child.Process == nil {
			return
		}
		// Negative pid → the whole process group, SIGTERM first.
		syscall.Kill(-child.Process.Pid, syscall.SIGTERM)
		done := make(chan struct{})
		go func() { child.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			syscall.Kill(-child.Process.Pid, syscall.SIGKILL)
			<-done
		}
		child = nil
	}

	if err := build(); err != nil {
		return fmt.Errorf("initial build failed")
	}
	start()
	step("running %s — watching for changes", dim("(ctrl-c to stop)"))

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	last := snapshot(dir)
	tick := time.NewTicker(400 * time.Millisecond)
	defer tick.Stop()

	for {
		select {
		case <-sig:
			fmt.Println()
			stop()
			return nil
		case <-tick.C:
			now := snapshot(dir)
			if now == last {
				continue
			}
			last = now
			fmt.Printf("%s rebuilding...\n", cyan("↻"))
			if err := build(); err != nil {
				fail("build failed — still serving the previous binary")
				continue
			}
			stop()
			start()
			step("restarted")
		}
	}
}

// snapshot fingerprints all .go files (path + mtime + size) cheaply.
func snapshot(dir string) string {
	var b strings.Builder
	filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == ".mgo" || strings.HasPrefix(name, ".") && name != "." {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		if info, err := d.Info(); err == nil {
			fmt.Fprintf(&b, "%s|%d|%d\n", path, info.ModTime().UnixNano(), info.Size())
		}
		return nil
	})
	return b.String()
}
