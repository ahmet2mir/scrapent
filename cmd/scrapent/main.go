package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/log"
	"github.com/joho/godotenv"
	"github.com/urfave/cli/v3"

	"github.com/ahmet2mir/scrapent/internal/version"
	"github.com/ahmet2mir/scrapent/pkg/scrapent"
)

func main() {
	_ = godotenv.Load()

	logger := log.NewWithOptions(os.Stderr, log.Options{ReportTimestamp: true})

	cmd := &cli.Command{
		Name:    version.ProgName,
		Usage:   "Archive blog posts and images from an ENT (entcore) space",
		Version: version.Version,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "login",
				Usage:   "ENT account login",
				Sources: cli.EnvVars("SCRAPENT_LOGIN"),
			},
			&cli.StringFlag{
				Name:    "password",
				Usage:   "ENT account password",
				Sources: cli.EnvVars("SCRAPENT_PASSWORD"),
			},
			&cli.StringFlag{
				Name:    "domain",
				Usage:   "ENT domain, e.g. ent-ecoles.ac-xxxxxxxxx.fr",
				Sources: cli.EnvVars("SCRAPENT_DOMAIN"),
			},
			&cli.StringFlag{
				Name:    "output",
				Aliases: []string{"o"},
				Usage:   "output directory",
				Value:   "dist",
				Sources: cli.EnvVars("SCRAPENT_OUTPUT"),
			},
		},
		Commands: []*cli.Command{
			{
				Name:  "blog",
				Usage: "list or download blogs",
				Commands: []*cli.Command{
					{
						Name:  "list",
						Usage: "list available blogs",
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:    "format",
								Aliases: []string{"f"},
								Usage:   "output format: terminal or json",
								Value:   "terminal",
							},
						},
						Action: blogList(logger),
					},
					{
						Name:  "get",
						Usage: "download one or more blogs",
						Flags: []cli.Flag{
							&cli.StringSliceFlag{
								Name:     "id",
								Usage:    "blog to download as name:id (repeatable); a bare id is used as its own name",
								Required: true,
							},
							&cli.BoolFlag{Name: "force-articles", Usage: "re-download article content even if present"},
							&cli.BoolFlag{Name: "skip-articles", Usage: "never download article content"},
							&cli.BoolFlag{Name: "force-images", Usage: "re-download images even if present"},
							&cli.BoolFlag{Name: "skip-images", Usage: "never download images"},
							&cli.BoolFlag{Name: "force-pdf", Usage: "regenerate PDFs even if present"},
							&cli.BoolFlag{Name: "skip-pdf", Usage: "never generate or merge PDFs"},
						},
						Action: blogGet(logger),
					},
					{
						Name:  "generate",
						Usage: "regenerate PDFs from a local data directory (no authentication)",
						Flags: []cli.Flag{
							&cli.StringSliceFlag{
								Name:  "name",
								Usage: "only regenerate these blog directory names (default: all under --output)",
							},
							&cli.BoolFlag{Name: "force", Usage: "regenerate article PDFs even if present"},
							&cli.BoolFlag{Name: "legacy", Usage: "convert legacy content.json files on the fly"},
						},
						Action: blogGenerate(logger),
					},
				},
			},
			{
				Name:      "migrate",
				Usage:     "convert a legacy content.json to the current format",
				ArgsUsage: "<content.json>",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:    "in-place",
						Aliases: []string{"i"},
						Usage:   "rewrite the file in place instead of printing to stdout",
					},
				},
				Action: migrate,
			},
		},
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		logger.Fatal(err)
	}
}

func newClient(cmd *cli.Command, logger *log.Logger) (*scrapent.Client, error) {
	login, password, domain := cmd.String("login"), cmd.String("password"), cmd.String("domain")
	if login == "" || password == "" || domain == "" {
		return nil, fmt.Errorf("--login, --password and --domain are required (or SCRAPENT_LOGIN/PASSWORD/DOMAIN)")
	}
	return scrapent.NewClient(login, password, domain, logger)
}

func blogGenerate(logger *log.Logger) cli.ActionFunc {
	return func(_ context.Context, cmd *cli.Command) error {
		return scrapent.GeneratePDFs(cmd.String("output"), cmd.StringSlice("name"), cmd.Bool("force"), cmd.Bool("legacy"), logger)
	}
}

func migrate(_ context.Context, cmd *cli.Command) error {
	if cmd.Args().Len() != 1 {
		return fmt.Errorf("usage: %s migrate [--in-place] <content.json>", version.ProgName)
	}
	path := cmd.Args().First()

	data, err := os.ReadFile(path) // #nosec G304 G703 -- path is the user-provided file to migrate
	if err != nil {
		return err
	}

	post, err := scrapent.MigrateLegacy(data)
	if err != nil {
		return err
	}

	out, err := json.MarshalIndent(post, "", "    ")
	if err != nil {
		return err
	}

	if cmd.Bool("in-place") {
		// #nosec G304 G703 -- path is the user-provided file to migrate in place (like sed -i)
		return os.WriteFile(path, append(out, '\n'), 0600)
	}

	fmt.Println(string(out))
	return nil
}

func blogList(logger *log.Logger) cli.ActionFunc {
	return func(_ context.Context, cmd *cli.Command) error {
		client, err := newClient(cmd, logger)
		if err != nil {
			return err
		}

		blogs, err := client.ListBlogs()
		if err != nil {
			return err
		}

		switch cmd.String("format") {
		case "json":
			return printBlogsJSON(blogs)
		case "terminal", "":
			printBlogsTerminal(blogs)
			return nil
		default:
			return fmt.Errorf("unknown format %q (want terminal or json)", cmd.String("format"))
		}
	}
}

func blogGet(logger *log.Logger) cli.ActionFunc {
	return func(_ context.Context, cmd *cli.Command) error {
		blogs := map[string]string{}
		for _, entry := range cmd.StringSlice("id") {
			name, id, ok := strings.Cut(strings.TrimSpace(entry), ":")
			if !ok {
				// No ':' separator: the value is a bare id, used as-is as its
				// own folder name.
				id = name
			}
			name = strings.TrimSpace(name)
			id = strings.TrimSpace(id)
			if id == "" {
				continue
			}
			blogs[name] = id
		}
		if len(blogs) == 0 {
			return fmt.Errorf("no valid --id provided")
		}

		client, err := newClient(cmd, logger)
		if err != nil {
			return err
		}

		opts := scrapent.Options{
			ForceArticles: cmd.Bool("force-articles"),
			SkipArticles:  cmd.Bool("skip-articles"),
			ForceImages:   cmd.Bool("force-images"),
			SkipImages:    cmd.Bool("skip-images"),
			ForcePDF:      cmd.Bool("force-pdf"),
			SkipPDF:       cmd.Bool("skip-pdf"),
		}

		return client.Scrape(blogs, cmd.String("output"), opts)
	}
}
