package scrapent

import (
	"encoding/json"
	"time"
)

// Article is a blog post as returned by the post list endpoint. Only the ID is
// used to fetch the full post; the other fields document the payload.
type Article struct {
	ID      string    `json:"_id"`
	Created MongoDate `json:"created"`
	Title   string    `json:"title"`
	Content string    `json:"content"`
}

// Post is the full blog post returned by the post detail endpoint.
type Post struct {
	ID               string      `json:"_id"`
	Author           Author      `json:"author"`
	Content          string      `json:"content"` // raw HTML
	ContentVersion   int         `json:"contentVersion"`
	Created          MongoDate   `json:"created"`
	FirstPublishDate MongoDate   `json:"firstPublishDate"`
	JSONContent      JSONContent `json:"jsonContent"`
	Modified         MongoDate   `json:"modified"`
	State            string      `json:"state"`
	Title            string      `json:"title"`
}

// Author describes the post's author.
type Author struct {
	Login    string `json:"login"`
	UserID   string `json:"userId"`
	Username string `json:"username"`
}

// MongoDate models Mongo's extended-JSON date wrapper: {"$date": "..."}.
type MongoDate struct {
	Date time.Time `json:"$date"`
}

// JSONContent is the ProseMirror/Tiptap-style document tree.
type JSONContent struct {
	Content []DocNode `json:"content"`
	Type    string    `json:"type"` // "doc"
}

// DocNode is a block-level node (only "paragraph" appears in this payload).
type DocNode struct {
	Attrs   *ParagraphAttrs `json:"attrs,omitempty"`
	Content []InlineNode    `json:"content,omitempty"`
	Type    string          `json:"type"` // "paragraph"
}

// ParagraphAttrs holds paragraph-level attributes.
type ParagraphAttrs struct {
	TextAlign *string `json:"textAlign"`
}

// InlineNode is a leaf node inside a paragraph: "text", "hardBreak", or
// "custom-image". Fields are optional depending on Type.
type InlineNode struct {
	Type  string      `json:"type"`
	Text  string      `json:"text,omitempty"`  // present when Type == "text"
	Attrs *ImageAttrs `json:"attrs,omitempty"` // present when Type == "custom-image"
}

// ImageAttrs holds attributes for a "custom-image" node. Only Src is used;
// the other attributes are loosely typed in the source (a value may be a
// string, null or an object), so they are kept as raw JSON to parse robustly.
type ImageAttrs struct {
	Src       string          `json:"src"`
	Alt       json.RawMessage `json:"alt"`
	Height    json.RawMessage `json:"height"`
	Size      json.RawMessage `json:"size"`
	Style     json.RawMessage `json:"style"`
	TextAlign json.RawMessage `json:"textAlign"`
	Title     json.RawMessage `json:"title"`
	Width     json.RawMessage `json:"width"`
}
