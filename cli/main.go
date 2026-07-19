// Command mgo is the MGO CLI: scaffold, evolve, and run MGO applications.
//
// Design position (docs 06/07): mgo is a flagship product, not an
// afterthought. Golden path in one command, generated code is native
// library code, and the CLI never overwrites files the user has edited.
package main

import (
	"os"

	"github.com/spf13/cobra"
)

const version = "0.5.0-dev"

func main() {
	root := &cobra.Command{
		Use:   "mgo",
		Short: "MGO — the Go application platform",
		Long: `mgo scaffolds and evolves MGO applications.

MGO is glue, not a cage: your routes are chi/stdlib routes, your queries
are GORM/SQL/ent queries, your handlers are net/http. mgo generates that
native code, keeps a manifest of what it generated, and refuses to
overwrite anything you have edited.`,
		Example: `  mgo new blog                      # interactive wizard
  mgo new api --preset api          # chi + gorm, no questions
  mgo dev                           # run with hot reload
  mgo make handler posts            # generate a handler + test
  mgo swap router stdmux            # change stack axes safely`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.Version = version

	root.AddCommand(newCmd(), swapCmd(), addCmd(), removeCmd(), makeCmd(), devCmd(), infoCmd(), diffCmd(), doctorCmd())

	if err := root.Execute(); err != nil {
		fail("%v", err)
		os.Exit(1)
	}
}
