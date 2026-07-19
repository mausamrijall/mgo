package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mgo-framework/mgo/cli/internal/scaffold"
	"github.com/spf13/cobra"
)

func makeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "make",
		Short: "Generate application code (handler, provider)",
	}
	cmd.AddCommand(makeHandlerCmd(), makeProviderCmd())
	return cmd
}

func makeHandlerCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "handler <name>",
		Short:   "Generate a net/http handler and its test",
		Example: `  mgo make handler posts`,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := strings.ToLower(args[0])
			sym := camel(name)
			file := "handler_" + name + ".go"
			testFile := "handler_" + name + "_test.go"

			handler := fmt.Sprintf(`package main

import (
	"net/http"

	"github.com/mgo-framework/mgo/framework/web"
)

// %[1]sHandler — wire it into router.go with your router's native API.
func %[1]sHandler(w http.ResponseWriter, r *http.Request) {
	web.JSON(w, http.StatusOK, map[string]string{"handler": "%[2]s"})
}
`, sym, name)

			test := fmt.Sprintf(`package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func Test%[3]sHandler(t *testing.T) {
	rec := httptest.NewRecorder()
	%[1]sHandler(rec, httptest.NewRequest("GET", "/%[2]s", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %%d, want 200", rec.Code)
	}
}
`, sym, name, export(sym))

			return writeNew(map[string]string{file: handler, testFile: test})
		},
	}
}

func makeProviderCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "provider <name>",
		Short:   "Generate an app lifecycle provider (Register/Boot/Close)",
		Example: `  mgo make provider cache`,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := strings.ToLower(args[0])
			sym := export(camel(name))
			file := "provider_" + name + ".go"

			provider := fmt.Sprintf(`package main

import (
	"context"

	appc "github.com/mgo-framework/mgo/contracts/app"
)

// %[1]sProvider joins the app lifecycle. Register binds services into the
// container (resolve nothing here); Boot runs after every provider has
// registered; Close runs on shutdown in reverse boot order.
// Add it in main.go: mgo.WithProviders(..., &%[2]sProvider{}).
type %[1]sProvider struct{}

func (p *%[1]sProvider) Register(app appc.App) error {
	return nil
}

func (p *%[1]sProvider) Boot(ctx context.Context, app appc.App) error {
	return nil
}

func (p *%[1]sProvider) Close(ctx context.Context) error {
	return nil
}
`, sym, name)

			return writeNew(map[string]string{file: provider})
		},
	}
}

// writeNew writes files that must not already exist, inside a project.
func writeNew(files map[string]string) error {
	dir, err := os.Getwd()
	if err != nil {
		return err
	}
	if _, err := scaffold.Load(dir); err != nil {
		return err
	}
	for path := range files {
		if _, err := os.Stat(filepath.Join(dir, path)); err == nil {
			return fmt.Errorf("%s already exists", path)
		}
	}
	for path, content := range files {
		if err := os.WriteFile(filepath.Join(dir, path), []byte(content), 0o644); err != nil {
			return err
		}
		step("created %s", path)
	}
	return nil
}

// camel turns snake/kebab into lowerCamel: user-posts → userPosts.
func camel(s string) string {
	parts := strings.FieldsFunc(s, func(r rune) bool { return r == '-' || r == '_' })
	if len(parts) == 0 {
		return s
	}
	out := parts[0]
	for _, p := range parts[1:] {
		out += export(p)
	}
	return out
}

// export upper-cases the first letter.
func export(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
