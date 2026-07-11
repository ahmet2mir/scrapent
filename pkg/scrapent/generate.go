package scrapent

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/charmbracelet/log"
)

// GeneratePDFs (re)builds each article PDF and the merged blog PDF for blogs
// found under outputDir, reading only local content.json files and images (no
// network, no authentication). When names is non-empty, only those blog
// directory names are processed; otherwise every blog directory is.
func GeneratePDFs(outputDir string, names []string, force bool, logger *log.Logger) error {
	entries, err := os.ReadDir(outputDir)
	if err != nil {
		return err
	}

	want := make(map[string]bool, len(names))
	for _, n := range names {
		want[n] = true
	}

	for _, e := range entries {
		if !e.IsDir() || (len(want) > 0 && !want[e.Name()]) {
			continue
		}
		generateBlogPDFs(filepath.Join(outputDir, e.Name()), force, logger)
	}

	return nil
}

// generateBlogPDFs builds the article PDFs and the merged PDF for a single blog
// directory from its local content.json files.
func generateBlogPDFs(blogDir string, force bool, logger *log.Logger) {
	entries, err := os.ReadDir(blogDir)
	if err != nil {
		logger.Error("Reading blog directory", "dir", blogDir, "err", err)
		return
	}

	logger.Info("Generating blog", "blog", filepath.Base(blogDir))

	var articlePDFs []datedPDF
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}

		articleDir := filepath.Join(blogDir, e.Name())
		contentPath := filepath.Join(articleDir, "content.json")

		// #nosec G304 -- contentPath is built from the output directory listing.
		data, err := os.ReadFile(contentPath)
		if err != nil {
			continue // not an article directory
		}

		var post Post
		if err := json.Unmarshal(data, &post); err != nil {
			logger.Error("Reading content", "dir", articleDir, "err", err)
			continue
		}

		if pdfPath, ok := buildArticlePDF(articleDir, &post, force, logger); ok {
			articlePDFs = append(articlePDFs, datedPDF{date: post.Created.Date, path: pdfPath})
		}
	}

	mergeBlogPDFs(blogDir, articlePDFs, logger)
}
