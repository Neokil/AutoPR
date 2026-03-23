package markdown

import (
	"path/filepath"
	"regexp"
	"strings"
)

var markdownLinkPattern = regexp.MustCompile(`\[(.*?)\]\(([^)\s]+)\)`)

func NormalizeRepoLinks(content string, roots ...string) string {
	if strings.TrimSpace(content) == "" {
		return content
	}
	cleanRoots := make([]string, 0, len(roots))
	for _, root := range roots {
		root = filepath.Clean(strings.TrimSpace(root))
		if root == "." || root == "" {
			continue
		}
		cleanRoots = append(cleanRoots, root)
	}
	if len(cleanRoots) == 0 {
		return content
	}
	return markdownLinkPattern.ReplaceAllStringFunc(content, func(match string) string {
		parts := markdownLinkPattern.FindStringSubmatch(match)
		if len(parts) != 3 {
			return match
		}
		target := normalizeMarkdownTarget(parts[2], cleanRoots)
		return "[" + parts[1] + "](" + target + ")"
	})
}

func normalizeMarkdownTarget(target string, roots []string) string {
	anchor := ""
	if idx := strings.Index(target, "#"); idx >= 0 {
		anchor = target[idx:]
		target = target[:idx]
	}
	if !filepath.IsAbs(target) {
		return target + anchor
	}
	cleanTarget := filepath.Clean(target)
	for _, root := range roots {
		rel, err := filepath.Rel(root, cleanTarget)
		if err != nil {
			continue
		}
		if rel == "." {
			return "." + anchor
		}
		if strings.HasPrefix(rel, "..") {
			continue
		}
		return filepath.ToSlash(rel) + anchor
	}
	return cleanTarget + anchor
}
