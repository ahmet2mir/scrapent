package scrapent

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

// Options controls, per resource kind, whether to force a re-download/regenerate
// even when the target already exists, or to skip that kind entirely.
type Options struct {
	ForceArticles bool
	SkipArticles  bool
	ForceImages   bool
	SkipImages    bool
	ForcePDF      bool
	SkipPDF       bool
}

// Scrape downloads every blog in blogs (name -> blog id) into outputDir. Each
// blog is stored under outputDir/<name>/<articleID>/.
func (c *Client) Scrape(blogs map[string]string, outputDir string, opts Options) error {
	timestamp := time.Now().UnixNano() / int64(time.Millisecond)

	for name, blogID := range blogs {
		blogID = strings.TrimSpace(blogID)
		if blogID == "" {
			continue
		}

		articles, err := c.GetArticles(blogID, timestamp)
		if err != nil {
			c.log.Error("Getting articles", "blog", blogID, "err", err)
			continue
		}

		if err := c.Download(name, blogID, articles, outputDir, opts); err != nil {
			c.log.Error("Downloading articles", "blog", blogID, "err", err)
		}
	}

	return nil
}

// GetArticles pages through the blog post list and returns every published post.
func (c *Client) GetArticles(blogID string, timestamp int64) ([]Article, error) {
	var articles []Article
	c.log.Info("Getting all articles", "blog", blogID)

	for i := 0; i < 1000; i++ {
		reqURL := fmt.Sprintf("https://%s/blog/post/list/all/%s?page=%d&content=true&comments=false&nbComments=true&states=PUBLISHED&_=%d", c.domain, blogID, i, timestamp)

		req, err := http.NewRequest("GET", reqURL, nil)
		if err != nil {
			return nil, fmt.Errorf("new request: %w", err)
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("do request: %w", err)
		}

		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("read body: %w", err)
		}

		var pageArticles []Article
		if err := json.Unmarshal(body, &pageArticles); err != nil {
			return nil, fmt.Errorf("unmarshal page %d (status %d): %w", i, resp.StatusCode, err)
		}

		if len(pageArticles) == 0 {
			break
		}

		articles = append(articles, pageArticles...)
	}

	return articles, nil
}

// Download saves each article's content.json and images under
// outputDir/<blogName>/<articleID>/. Articles whose content.json already exists
// are not re-fetched, and images already on disk are not re-downloaded.
func (c *Client) Download(blogName, blogID string, articles []Article, outputDir string, opts Options) error {
	c.log.Info("Downloading each article", "blog", blogName, "count", len(articles))

	blogDir := filepath.Join(outputDir, blogName)
	if err := os.MkdirAll(blogDir, 0750); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(blogDir, ".id"), []byte(blogID), 0600); err != nil {
		return err
	}

	var articlePDFs []datedPDF

	for _, article := range articles {
		articleID := article.ID

		created := "unknown"
		if !article.Created.Date.IsZero() {
			created = article.Created.Date.Format("2006-01-02")
		}

		articleDir := filepath.Join(blogDir, fmt.Sprintf("%s-%s", created, SafeName(article.Title)))
		contentPath := filepath.Join(articleDir, "content.json")

		post, err := c.article(blogID, articleID, articleDir, contentPath, opts)
		if err != nil {
			c.log.Error("Fetching article", "article", articleID, "err", err)
			continue
		}
		if post == nil {
			continue
		}

		prefix := "unknown"
		if !post.Created.Date.IsZero() {
			prefix = post.Created.Date.Format("2006-01-02-15-04-05")
		}

		if !opts.SkipImages {
			c.downloadImages(post, articleDir, articleID, prefix, opts.ForceImages)
		}

		if !opts.SkipPDF {
			if pdfPath, ok := buildArticlePDF(articleDir, post, opts.ForcePDF, c.log); ok {
				articlePDFs = append(articlePDFs, datedPDF{date: post.Created.Date, path: pdfPath})
			}
		}
	}

	if !opts.SkipPDF {
		mergeBlogPDFs(blogDir, articlePDFs, c.log)
	}

	return nil
}

// datedPDF pairs an article PDF path with its date for chronological merging.
type datedPDF struct {
	date time.Time
	path string
}

// article returns the post for one article honouring the article options. It
// returns (nil, nil) when the article should be skipped (skip requested and no
// content.json present).
func (c *Client) article(blogID, articleID, articleDir, contentPath string, opts Options) (*Post, error) {
	_, statErr := os.Stat(contentPath)
	exists := statErr == nil

	if opts.SkipArticles {
		if !exists {
			return nil, nil
		}
		return c.loadArticle(contentPath, articleID)
	}

	if exists && !opts.ForceArticles {
		return c.loadArticle(contentPath, articleID)
	}

	return c.fetchArticle(blogID, articleID, articleDir, contentPath)
}

// loadArticle reads a post from an existing content.json.
func (c *Client) loadArticle(contentPath, articleID string) (*Post, error) {
	// #nosec G304 G703 -- contentPath is built from outputDir and program-sanitized
	// blog/article names, not from external input.
	cached, err := os.ReadFile(contentPath)
	if err != nil {
		return nil, err
	}

	var post Post
	if err := json.Unmarshal(cached, &post); err != nil {
		return nil, fmt.Errorf("read cached content: %w", err)
	}
	c.log.Info("Article already downloaded, skipping fetch", "article", articleID)
	return &post, nil
}

// fetchArticle downloads the full post and writes content.json (and .id).
func (c *Client) fetchArticle(blogID, articleID, articleDir, contentPath string) (*Post, error) {
	reqURL := fmt.Sprintf("https://%s/blog/post/%s/%s", c.domain, blogID, articleID)
	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		return nil, err
	}

	var post Post
	if err := json.Unmarshal(body, &post); err != nil {
		return nil, fmt.Errorf("unmarshal (status %d): %w", resp.StatusCode, err)
	}

	if err := os.MkdirAll(articleDir, 0750); err != nil {
		return nil, err
	}

	if err := os.WriteFile(filepath.Join(articleDir, ".id"), []byte(articleID), 0600); err != nil {
		return nil, err
	}

	out, err := json.MarshalIndent(post, "", "    ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(contentPath, out, 0600); err != nil {
		return nil, err
	}

	c.log.Info("Article saved", "dir", articleDir)
	return &post, nil
}

// downloadImages downloads each "custom-image" of the post into articleDir.
// Images already present are skipped unless force is set.
func (c *Client) downloadImages(post *Post, articleDir, articleID, prefix string, force bool) {
	// Images live in the structured jsonContent as "custom-image" inline nodes;
	// their src is "/workspace/document/<id>" without a query string.
	for _, node := range post.JSONContent.Content {
		for _, inline := range node.Content {
			if inline.Type != "custom-image" || inline.Attrs == nil || inline.Attrs.Src == "" {
				continue
			}

			src := inline.Attrs.Src
			// Only server document paths are downloadable; some posts carry a
			// transient "blob:" or absolute src that must be ignored.
			if !strings.HasPrefix(src, "/") {
				continue
			}

			imgFile := filepath.Join(articleDir, fmt.Sprintf("%s--%s.jpg", prefix, path.Base(src)))
			if _, err := os.Stat(imgFile); err == nil && !force {
				continue
			}

			if err := c.downloadImage(src, imgFile); err != nil {
				c.log.Error("Downloading image", "article", articleID, "src", src, "err", err)
			}
		}
	}
}

// accentReplacer maps common French (and a few related) accented letters to
// their ASCII equivalent before slugifying.
var accentReplacer = strings.NewReplacer(
	"à", "a", "â", "a", "ä", "a", "á", "a", "ã", "a", "å", "a",
	"ç", "c",
	"è", "e", "é", "e", "ê", "e", "ë", "e",
	"ì", "i", "î", "i", "ï", "i", "í", "i",
	"ò", "o", "ô", "o", "ö", "o", "ó", "o", "õ", "o",
	"ù", "u", "û", "u", "ü", "u", "ú", "u",
	"ý", "y", "ÿ", "y",
	"ñ", "n",
	"œ", "oe", "æ", "ae",
	"À", "A", "Â", "A", "Ä", "A", "Á", "A", "Ã", "A", "Å", "A",
	"Ç", "C",
	"È", "E", "É", "E", "Ê", "E", "Ë", "E",
	"Ì", "I", "Î", "I", "Ï", "I", "Í", "I",
	"Ò", "O", "Ô", "O", "Ö", "O", "Ó", "O", "Õ", "O",
	"Ù", "U", "Û", "U", "Ü", "U", "Ú", "U",
	"Ý", "Y", "Ÿ", "Y",
	"Ñ", "N",
	"Œ", "OE", "Æ", "AE",
)

// SafeName slugifies an article title into a filesystem-safe directory name:
// French accents are transliterated to ASCII, spaces become dashes, the result
// is lowercased, and any remaining non-alphanumeric character (emoji,
// punctuation, ...) is dropped.
func SafeName(s string) string {
	s = accentReplacer.Replace(s)

	var b strings.Builder
	lastDash := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		case r == ' ' || r == '-' || r == '_':
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}

	out := strings.ToLower(strings.TrimRight(b.String(), "-"))
	if out == "" {
		return "untitled"
	}
	return out
}

// downloadImage fetches a "/workspace/document/<id>" image at full resolution
// and writes it to dst.
func (c *Client) downloadImage(src, dst string) error {
	imgURL := fmt.Sprintf("https://%s%s?thumbnail=16384x0", c.domain, src)
	c.log.Info("Downloading image", "url", imgURL)

	req, err := http.NewRequest("GET", imgURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "image/jpeg,image/jpg")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	return os.WriteFile(dst, data, 0600)
}
