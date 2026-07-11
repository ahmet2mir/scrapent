package scrapent

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// BlogInfo describes a blog the account can access.
type BlogInfo struct {
	ID          string
	Title       string
	Description string
}

// Name is the slugified title, matching the article directory naming.
func (b BlogInfo) Name() string {
	return SafeName(b.Title)
}

type explorerResource struct {
	AssetID     string `json:"assetId"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type explorerResponse struct {
	Resources  []explorerResource `json:"resources"`
	Pagination struct {
		StartIdx int `json:"startIdx"`
		PageSize int `json:"pageSize"`
		MaxIdx   int `json:"maxIdx"`
	} `json:"pagination"`
}

// ListBlogs returns every blog the account can access, paging through the
// explorer resource listing.
func (c *Client) ListBlogs() ([]BlogInfo, error) {
	const pageSize = 48
	var blogs []BlogInfo

	for start := 0; ; start += pageSize {
		reqURL := fmt.Sprintf("https://%s/explorer/resources?application=blog&start_idx=%d&page_size=%d&trashed=false&resource_type=blog&order_by=updatedAt:desc&folder=default", c.domain, start, pageSize)

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

		var er explorerResponse
		if err := json.Unmarshal(body, &er); err != nil {
			return nil, fmt.Errorf("unmarshal (status %d): %w", resp.StatusCode, err)
		}

		for _, r := range er.Resources {
			blogs = append(blogs, BlogInfo{ID: r.AssetID, Title: r.Name, Description: r.Description})
		}

		if len(er.Resources) == 0 || start+pageSize >= er.Pagination.MaxIdx {
			break
		}
	}

	return blogs, nil
}
