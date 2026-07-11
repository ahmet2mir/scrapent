package scrapent

import (
	"encoding/json"
	"strings"
	"time"

	"golang.org/x/net/html"
)

// legacyDate parses the legacy {"$date": <unix-millis>} form, where the date is
// a number rather than the RFC3339 string used by the current API.
type legacyDate struct {
	Millis int64
}

func (d *legacyDate) UnmarshalJSON(b []byte) error {
	var w struct {
		Date int64 `json:"$date"`
	}
	if err := json.Unmarshal(b, &w); err != nil {
		return err
	}
	d.Millis = w.Date
	return nil
}

func (d legacyDate) time() time.Time {
	if d.Millis == 0 {
		return time.Time{}
	}
	return time.UnixMilli(d.Millis).UTC()
}

// legacyPost is the old blog post shape: HTML content, millisecond dates and no
// structured jsonContent.
type legacyPost struct {
	ID               string     `json:"_id"`
	Content          string     `json:"content"`
	Title            string     `json:"title"`
	Created          legacyDate `json:"created"`
	Modified         legacyDate `json:"modified"`
	FirstPublishDate legacyDate `json:"firstPublishDate"`
	Author           Author     `json:"author"`
	State            string     `json:"state"`
}

// MigrateLegacy converts a legacy content.json into the current Post structure:
// dates become RFC3339 and the HTML content is parsed into structured
// jsonContent (text, hard breaks and custom-image nodes).
func MigrateLegacy(data []byte) (*Post, error) {
	var lp legacyPost
	if err := json.Unmarshal(data, &lp); err != nil {
		return nil, err
	}

	return &Post{
		ID:               lp.ID,
		Author:           lp.Author,
		Content:          lp.Content,
		Created:          MongoDate{Date: lp.Created.time()},
		FirstPublishDate: MongoDate{Date: lp.FirstPublishDate.time()},
		Modified:         MongoDate{Date: lp.Modified.time()},
		State:            lp.State,
		Title:            lp.Title,
		JSONContent:      htmlToJSONContent(lp.Content),
	}, nil
}

// htmlToJSONContent parses HTML into the ProseMirror-style document used by the
// current format: block elements become paragraphs, <img> becomes a
// custom-image node, and <br> becomes a hard break.
func htmlToJSONContent(htmlStr string) JSONContent {
	doc, err := html.Parse(strings.NewReader(htmlStr))
	if err != nil {
		return JSONContent{Type: "doc"}
	}

	var paragraphs []DocNode
	var cur []InlineNode
	flush := func() {
		if len(cur) > 0 {
			paragraphs = append(paragraphs, DocNode{Type: "paragraph", Content: cur})
			cur = nil
		}
	}

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		switch n.Type {
		case html.TextNode:
			if t := cleanText(n.Data); t != "" {
				cur = append(cur, InlineNode{Type: "text", Text: t})
			}
			return
		case html.ElementNode:
			switch n.Data {
			case "br":
				cur = append(cur, InlineNode{Type: "hardBreak"})
				return
			case "img":
				if src := stripQuery(attrValue(n, "src")); src != "" {
					cur = append(cur, InlineNode{Type: "custom-image", Attrs: &ImageAttrs{Src: src}})
				}
				return
			case "div", "p", "li", "ul", "ol", "h1", "h2", "h3", "h4", "h5", "h6", "blockquote", "table", "tr":
				// Block boundary: close the current paragraph around the block.
				flush()
				for c := n.FirstChild; c != nil; c = c.NextSibling {
					walk(c)
				}
				flush()
				return
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	flush()

	return JSONContent{Type: "doc", Content: paragraphs}
}

func attrValue(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

// stripQuery removes a "?..." suffix so an image src becomes a bare document
// path (the downloader adds its own thumbnail query).
func stripQuery(s string) string {
	if i := strings.IndexByte(s, '?'); i >= 0 {
		return s[:i]
	}
	return s
}

// cleanText drops zero-width and non-breaking spaces and collapses whitespace,
// returning "" for whitespace-only text.
func cleanText(s string) string {
	s = strings.ReplaceAll(s, "\u200b", "")  // zero-width space
	s = strings.ReplaceAll(s, "\ufeff", "")  // zero-width no-break space / BOM
	s = strings.ReplaceAll(s, "\u00a0", " ") // non-breaking space
	return strings.Join(strings.Fields(s), " ")
}
