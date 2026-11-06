package pipeline

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"slices"
	"strings"

	"github.com/josepavese/needlex/internal/platform"
	"golang.org/x/net/html"
)

type downloadAsset struct {
	url  *url.URL
	kind string
}

type assetReference struct {
	address, kind string
}

func discoverDownloadAssets(ctx context.Context, directory platform.DownloadDirectory, page *url.URL, origins map[string]bool) ([]downloadAsset, []string, error) {
	source, err := directory.Open(downloadHTMLName)
	if err != nil {
		return nil, nil, err
	}
	defer platform.Close(source)

	base, references, err := scanDownloadAssetReferences(ctx, source, page)
	if err != nil {
		return nil, nil, err
	}

	var assets []downloadAsset
	var skipped []string
	seen := map[string]bool{page.String(): true}
	for _, ref := range references {
		resolved, err := base.Parse(ref.address)
		if err != nil {
			continue
		}
		target, err := downloadURL(resolved.String())
		if err != nil || seen[target.String()] {
			continue
		}
		seen[target.String()] = true
		origin := downloadOrigin(target)
		if !origins[origin] {
			if !slices.Contains(skipped, origin) {
				skipped = append(skipped, origin)
			}
			continue
		}
		assets = append(assets, downloadAsset{url: target, kind: ref.kind})
	}
	return assets, skipped, nil
}

func scanDownloadAssetReferences(ctx context.Context, source io.Reader, page *url.URL) (*url.URL, []assetReference, error) {
	// The first base href applies even to tags that precede it.
	base := page
	var hasBase bool
	var references []assetReference
	known := make(map[assetReference]bool)
	tokenizer := html.NewTokenizer(source)
	for {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		tokenType := tokenizer.Next()
		if tokenType == html.ErrorToken {
			if err := tokenizer.Err(); !errors.Is(err, io.EOF) {
				return nil, nil, err
			}
			break
		}
		if tokenType != html.StartTagToken && tokenType != html.SelfClosingTagToken {
			continue
		}
		token := tokenizer.Token()
		if token.Data == "base" && !hasBase {
			for _, attr := range token.Attr {
				if attr.Key == "href" {
					hasBase = true
					if resolved, err := page.Parse(strings.TrimSpace(attr.Val)); err == nil {
						base = resolved
					}
					break
				}
			}
		}
		ref := linkedDownloadAsset(token)
		if ref.address != "" && !known[ref] {
			if len(references) == MaxDownloadAssets {
				return nil, nil, fmt.Errorf("HTML exceeds the limit of %d distinct asset references", MaxDownloadAssets)
			}
			known[ref] = true
			references = append(references, ref)
		}
	}

	return base, references, nil
}

func linkedDownloadAsset(token html.Token) assetReference {
	var src, href, rel, as string
	for _, attr := range token.Attr {
		value := strings.TrimSpace(attr.Val)
		switch attr.Key {
		case "src":
			src = value
		case "href":
			href = value
		case "rel":
			rel = strings.ToLower(value)
		case "as":
			as = strings.ToLower(value)
		}
	}
	switch token.Data {
	case "script":
		return assetReference{src, "js"}
	case "link":
		rels := strings.Fields(rel)
		if slices.Contains(rels, "stylesheet") || (slices.Contains(rels, "preload") && as == "style") {
			return assetReference{href, "css"}
		}
		if slices.Contains(rels, "modulepreload") || (slices.Contains(rels, "preload") && as == "script") {
			return assetReference{href, "js"}
		}
	}
	return assetReference{}
}
