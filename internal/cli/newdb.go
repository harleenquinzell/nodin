package cli

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/harleenquinzell/nodin/internal/config"
	"github.com/harleenquinzell/nodin/internal/notion"
	"github.com/harleenquinzell/nodin/internal/state"
	internalsync "github.com/harleenquinzell/nodin/internal/sync"
)

func newNewDBCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "new-db",
		Short: "Create a new Notion database (interactive)",
		Long: `new-db walks you through creating a new Notion database.

It prompts for the database title, parent page, and each property
(name, type, and — for select / multi_select — options). It then
creates the database on Notion and writes databases/<slug>-<id>/_schema.json
locally so push can manage entries inside it.`,
		RunE: runNewDB,
	}
}

func runNewDB(cmd *cobra.Command, _ []string) error {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

	token, err := cfg.ResolvedToken()
	if err != nil {
		return fmt.Errorf("resolve token: %w", err)
	}

	store := state.Open(cfg.SyncDir)
	if err := store.Init(); err != nil {
		return fmt.Errorf("init state: %w", err)
	}

	r := bufio.NewReader(os.Stdin)
	schema, parentPageID, err := promptForSchema(r, cfg)
	if err != nil {
		return err
	}

	client := notion.NewClient(token, cfg.RPS)
	db, err := internalsync.CreateDatabase(cmd.Context(), cfg, store, client, schema,
		internalsync.CreateDatabaseOptions{ParentPageID: parentPageID})
	if err != nil {
		return err
	}

	cmd.Printf("created database: %s (id %s)\n", db.TitleText(), db.ID)
	return nil
}

// promptForSchema interactively collects the title, parent, and properties
// of a new database. It returns a validated DatabaseSchema and the chosen
// parent page ID (defaulting to cfg.RootPageID).
func promptForSchema(r *bufio.Reader, cfg *config.Config) (internalsync.DatabaseSchema, string, error) {
	fmt.Println("nodin new-db")
	fmt.Println()

	// title
	title, err := promptRequired(r, "Database title: ")
	if err != nil {
		return internalsync.DatabaseSchema{}, "", err
	}

	// parent
	prompt := "Parent Notion page ID"
	if cfg.RootPageID != "" {
		prompt += fmt.Sprintf(" [%s]", cfg.RootPageID)
	}
	prompt += ": "
	parentInput, err := promptLine(r, prompt)
	if err != nil {
		return internalsync.DatabaseSchema{}, "", err
	}
	parentPageID := parentInput
	if parentPageID == "" {
		parentPageID = cfg.RootPageID
	}
	if parentPageID == "" {
		return internalsync.DatabaseSchema{}, "", fmt.Errorf("parent page ID is required (no root_page_id in config)")
	}

	// properties
	fmt.Println()
	fmt.Println("Properties — one per row. Press Enter on an empty name to finish.")
	fmt.Printf("Supported types: %s\n", supportedTypesList())
	fmt.Println()

	props := map[string]internalsync.PropertySpec{}
	hasTitle := false
	for i := 1; ; i++ {
		fmt.Printf("Property %d\n", i)
		name, err := promptLine(r, "  Name (empty to finish): ")
		if err != nil {
			return internalsync.DatabaseSchema{}, "", err
		}
		if name == "" {
			break
		}
		if _, dup := props[name]; dup {
			fmt.Printf("  (already defined: %s — skipped)\n", name)
			continue
		}

		typ, err := promptPropertyType(r, hasTitle)
		if err != nil {
			return internalsync.DatabaseSchema{}, "", err
		}

		spec := internalsync.PropertySpec{Type: typ}
		switch typ {
		case "select", "multi_select":
			spec.Options, err = promptSelectOptions(r)
			if err != nil {
				return internalsync.DatabaseSchema{}, "", err
			}
		case "formula":
			spec.Expression, err = promptRequired(r, "  Formula expression: ")
			if err != nil {
				return internalsync.DatabaseSchema{}, "", err
			}
		case "relation":
			spec.RelationDatabaseID, err = promptRequired(r, "  Target database ID: ")
			if err != nil {
				return internalsync.DatabaseSchema{}, "", err
			}
		}

		if typ == "title" {
			hasTitle = true
		}
		props[name] = spec
	}

	if !hasTitle {
		return internalsync.DatabaseSchema{}, "", fmt.Errorf("at least one property must have type \"title\"")
	}

	schema := internalsync.DatabaseSchema{Title: title, Properties: props}
	if err := internalsync.ValidateSchema(schema); err != nil {
		return internalsync.DatabaseSchema{}, "", err
	}
	return schema, parentPageID, nil
}

func promptPropertyType(r *bufio.Reader, titleAlreadyDefined bool) (string, error) {
	for {
		typ, err := promptLine(r, "  Type: ")
		if err != nil {
			return "", err
		}
		if typ == "" {
			fmt.Println("  type is required")
			continue
		}
		if !internalsync.SupportedPropertyTypes[typ] {
			fmt.Printf("  unsupported type %q (supported: %s)\n", typ, supportedTypesList())
			continue
		}
		if typ == "title" && titleAlreadyDefined {
			fmt.Println("  a title property has already been defined; only one is allowed")
			continue
		}
		return typ, nil
	}
}

func promptSelectOptions(r *bufio.Reader) ([]internalsync.SelectOption, error) {
	fmt.Println("  Options — one per row. Press Enter on an empty name to finish.")
	var opts []internalsync.SelectOption
	for {
		name, err := promptLine(r, "    Option name (empty to finish): ")
		if err != nil {
			return nil, err
		}
		if name == "" {
			if len(opts) == 0 {
				fmt.Println("    at least one option is required for select / multi_select")
				continue
			}
			return opts, nil
		}
		color, err := promptLine(r, "    Color [default]: ")
		if err != nil {
			return nil, err
		}
		if color != "" && !internalsync.SupportedSelectColors[color] {
			fmt.Printf("    unsupported color %q — try one of: %s\n", color, supportedColorsList())
			continue
		}
		opts = append(opts, internalsync.SelectOption{Name: name, Color: color})
	}
}

func promptLine(r *bufio.Reader, prompt string) (string, error) {
	fmt.Print(prompt)
	line, err := r.ReadString('\n')
	if err != nil && line == "" {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

func promptRequired(r *bufio.Reader, prompt string) (string, error) {
	for {
		v, err := promptLine(r, prompt)
		if err != nil {
			return "", err
		}
		if v != "" {
			return v, nil
		}
		fmt.Println("  required")
	}
}

func supportedTypesList() string {
	out := make([]string, 0, len(internalsync.SupportedPropertyTypes))
	for t := range internalsync.SupportedPropertyTypes {
		out = append(out, t)
	}
	sort.Strings(out)
	return strings.Join(out, ", ")
}

func supportedColorsList() string {
	out := make([]string, 0, len(internalsync.SupportedSelectColors))
	for c := range internalsync.SupportedSelectColors {
		if c == "" {
			continue
		}
		out = append(out, c)
	}
	sort.Strings(out)
	return strings.Join(out, ", ")
}
