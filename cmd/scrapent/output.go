package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/list"

	"github.com/ahmet2mir/scrapent/pkg/scrapent"
)

var (
	blogTitleStyle  = lipgloss.NewStyle().Bold(true)
	blogIDStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	blogDescStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Italic(true)
	blogBulletStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("212")).MarginRight(1)
)

// printBlogsTerminal renders blogs as a styled bullet list.
func printBlogsTerminal(blogs []scrapent.BlogInfo) {
	l := list.New().Enumerator(list.Bullet).EnumeratorStyle(blogBulletStyle)

	for _, b := range blogs {
		item := blogTitleStyle.Render(b.Title) + "\n" + blogIDStyle.Render(b.Name()+":"+b.ID)
		if d := strings.Join(strings.Fields(b.Description), " "); d != "" {
			item += "\n" + blogDescStyle.Render(d)
		}
		l.Item(item)
	}

	fmt.Println(l)
}

// printBlogsJSON prints blogs as an indented JSON array.
func printBlogsJSON(blogs []scrapent.BlogInfo) error {
	type blogJSON struct {
		Name        string `json:"name"`
		ID          string `json:"id"`
		Title       string `json:"title"`
		Description string `json:"description"`
	}

	out := make([]blogJSON, 0, len(blogs))
	for _, b := range blogs {
		out = append(out, blogJSON{
			Name:        b.Name(),
			ID:          b.ID,
			Title:       b.Title,
			Description: strings.Join(strings.Fields(b.Description), " "),
		})
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}
