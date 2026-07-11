package scrapent

import (
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/log"
	"github.com/go-pdf/fpdf"
	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// buildArticlePDF renders the article PDF when it is missing, invalid or force
// is set, and reports its path plus whether a valid PDF is now present.
func buildArticlePDF(articleDir string, post *Post, force bool, logger *log.Logger) (string, bool) {
	pdfPath := filepath.Join(articleDir, filepath.Base(articleDir)+".pdf")
	if force || !validPDF(pdfPath) {
		if err := renderPDF(post, articleDir, pdfPath); err != nil {
			logger.Error("Generating PDF", "dir", articleDir, "err", err)
		} else {
			logger.Info("PDF generated", "path", pdfPath)
		}
	}

	return pdfPath, validPDF(pdfPath)
}

// mergeBlogPDFs merges the article PDFs, oldest first by date, into the
// blog-level PDF. A blog with no valid article PDFs produces nothing.
func mergeBlogPDFs(blogDir string, pdfs []datedPDF, logger *log.Logger) {
	if len(pdfs) == 0 {
		return
	}

	sort.Slice(pdfs, func(i, j int) bool {
		return pdfs[i].date.Before(pdfs[j].date)
	})
	paths := make([]string, len(pdfs))
	for i, e := range pdfs {
		paths[i] = e.path
	}

	mergedPath := filepath.Join(blogDir, filepath.Base(blogDir)+".pdf")
	if err := mergePDFs(paths, mergedPath); err != nil {
		logger.Error("Merging PDFs", "blog", filepath.Base(blogDir), "err", err)
	} else {
		logger.Info("Merged PDF", "path", mergedPath, "articles", len(paths))
	}
}

func init() {
	// Do not create or read pdfcpu's user config directory.
	model.ConfigPath = "disable"
}

// mergePDFs concatenates inFiles (already ordered by the caller) into dst.
func mergePDFs(inFiles []string, dst string) error {
	return api.MergeCreateFile(inFiles, dst, false, nil)
}

const (
	pageWidth    = 210.0 // A4 width in mm
	pageMargin   = 20.0
	contentWidth = pageWidth - 2*pageMargin
)

// renderPDF writes a document-style PDF of the post to dst. Text and layout come
// from the structured jsonContent; images are embedded from the files already
// downloaded next to the post under articleDir (named "<prefix>--<id>.jpg"),
// resolved by their document id so the date prefix does not have to match.
func renderPDF(post *Post, articleDir, dst string) error {
	images := indexArticleImages(articleDir)

	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(pageMargin, pageMargin, pageMargin)
	pdf.SetAutoPageBreak(true, pageMargin)

	// fpdf core fonts are cp1252; the translator maps French accents correctly.
	tr := pdf.UnicodeTranslatorFromDescriptor("")

	pdf.SetFooterFunc(func() {
		pdf.SetY(-15)
		pdf.SetFont("Helvetica", "", 8)
		pdf.SetTextColor(150, 150, 150)
		pdf.CellFormat(0, 10, fmt.Sprintf("%d", pdf.PageNo()), "", 0, "C", false, 0, "")
	})

	pdf.AddPage()

	// Title.
	pdf.SetFont("Helvetica", "B", 22)
	pdf.SetTextColor(33, 37, 41)
	title := strings.TrimSpace(post.Title)
	if title == "" {
		title = "Sans titre"
	}
	pdf.MultiCell(0, 10, tr(title), "", "L", false)

	// Metadata: date and author.
	var meta []string
	if !post.Created.Date.IsZero() {
		meta = append(meta, post.Created.Date.Format("02/01/2006"))
	}
	if author := strings.TrimSpace(post.Author.Username); author != "" {
		meta = append(meta, author)
	}
	if len(meta) > 0 {
		pdf.Ln(1)
		pdf.SetFont("Helvetica", "", 10)
		pdf.SetTextColor(130, 130, 130)
		pdf.MultiCell(0, 6, tr(strings.Join(meta, "  -  ")), "", "L", false)
	}

	// Divider.
	pdf.Ln(2)
	y := pdf.GetY()
	pdf.SetDrawColor(220, 220, 220)
	pdf.Line(pageMargin, y, pageWidth-pageMargin, y)
	pdf.Ln(6)

	// Body.
	for _, node := range post.JSONContent.Content {
		renderParagraph(pdf, tr, node, images)
	}

	// Write atomically so an interrupted run cannot leave an empty or partial
	// PDF that would later break the merge.
	tmp := dst + ".tmp"
	if err := pdf.OutputFileAndClose(tmp); err != nil {
		return err
	}
	return os.Rename(tmp, dst)
}

// validPDF reports whether path is an existing, non-empty file starting with
// the PDF header. Empty or corrupt files (e.g. from an interrupted run) are
// treated as invalid so they get regenerated instead of breaking the merge.
func validPDF(path string) bool {
	f, err := os.Open(path) // #nosec G304 G703 -- path is program-built under outputDir
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()

	buf := make([]byte, 5)
	n, _ := io.ReadFull(f, buf)
	return n == 5 && string(buf) == "%PDF-"
}

// indexArticleImages maps each downloaded image's document id to its path, so
// images can be found by id regardless of the date prefix in the filename
// ("<prefix>--<id>.jpg").
func indexArticleImages(articleDir string) map[string]string {
	index := map[string]string{}
	entries, err := os.ReadDir(articleDir)
	if err != nil {
		return index
	}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".jpg") {
			continue
		}
		if i := strings.LastIndex(name, "--"); i >= 0 {
			id := strings.TrimSuffix(name[i+2:], ".jpg")
			index[id] = filepath.Join(articleDir, name)
		}
	}
	return index
}

// renderParagraph renders one block node: its text runs (honouring alignment)
// followed by any images it contains.
func renderParagraph(pdf *fpdf.Fpdf, tr func(string) string, node DocNode, images map[string]string) {
	align := "L"
	if node.Attrs != nil && node.Attrs.TextAlign != nil {
		switch *node.Attrs.TextAlign {
		case "center":
			align = "C"
		case "right":
			align = "R"
		case "justify":
			align = "J"
		}
	}

	var sb strings.Builder
	var imgSrcs []string
	for _, inline := range node.Content {
		switch inline.Type {
		case "text":
			sb.WriteString(inline.Text)
		case "hardBreak":
			sb.WriteString("\n")
		case "custom-image":
			if inline.Attrs != nil && inline.Attrs.Src != "" {
				imgSrcs = append(imgSrcs, inline.Attrs.Src)
			}
		}
	}

	if text := strings.TrimSpace(sb.String()); text != "" {
		pdf.SetFont("Helvetica", "", 11)
		pdf.SetTextColor(40, 40, 40)
		pdf.MultiCell(0, 6, tr(text), "", align, false)
		pdf.Ln(2)
	}

	for _, src := range imgSrcs {
		if p := images[path.Base(src)]; p != "" {
			placeImage(pdf, p)
		}
	}
}

// placeImage embeds a local image, centered and scaled to fit the content width
// without upscaling beyond its natural size. Files that are missing, unreadable
// or not a decodable image are skipped so a bad file never breaks the PDF.
func placeImage(pdf *fpdf.Fpdf, imgPath string) {
	imgType := imageType(imgPath)
	if imgType == "" {
		return
	}

	opt := fpdf.ImageOptions{ImageType: imgType, ReadDpi: true}
	info := pdf.RegisterImageOptions(imgPath, opt)
	if info == nil {
		return
	}

	w := contentWidth
	if natural := info.Width() * 25.4 / 72.0; natural > 0 && natural < contentWidth {
		w = natural
	}
	x := (pageWidth - w) / 2.0

	pdf.ImageOptions(imgPath, x, pdf.GetY(), w, 0, true, opt, 0, "")
	pdf.Ln(4)
}

// imageType returns the fpdf image type ("JPG", "PNG" or "GIF") for a local
// file, or "" if it is missing or not a recognised image. Detection is by
// content, since downloaded files are always named ".jpg".
func imageType(imgPath string) string {
	f, err := os.Open(imgPath) // #nosec G304 G703 -- imgPath is program-built under outputDir
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()

	_, format, err := image.DecodeConfig(f)
	if err != nil {
		return ""
	}
	switch format {
	case "jpeg":
		return "JPG"
	case "png":
		return "PNG"
	case "gif":
		return "GIF"
	default:
		return ""
	}
}
