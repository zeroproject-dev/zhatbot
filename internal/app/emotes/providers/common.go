package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"zhatBot/internal/app/emotes"
)

const userAgent = "zhatBot/third-party-emotes (+https://github.com/zero-24/zhatBot)"

type httpClient interface {
	Do(req *http.Request) (*http.Response, error)
}

func ensureClient(cli *http.Client) *http.Client {
	if cli != nil {
		return cli
	}
	return &http.Client{
		Timeout: 5 * time.Second,
	}
}

func doJSON(ctx context.Context, cli httpClient, method, url string, dest interface{}) error {
	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := cli.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<14))
		return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	if dest == nil {
		return nil
	}

	return json.NewDecoder(resp.Body).Decode(dest)
}

func normalizeURLs(urls emotes.EmoteURLs) emotes.EmoteURLs {
	if urls.Small == "" {
		urls.Small = firstNonEmpty(urls.Medium, urls.Large)
	}
	if urls.Medium == "" {
		urls.Medium = firstNonEmpty(urls.Large, urls.Small)
	}
	if urls.Large == "" {
		urls.Large = firstNonEmpty(urls.Medium, urls.Small)
	}
	return urls
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func absoluteURL(value string) string {
	if value == "" {
		return ""
	}
	if strings.HasPrefix(value, "//") {
		return "https:" + value
	}
	return value
}
