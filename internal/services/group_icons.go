package services

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const iconifyAPIBase = "https://api.iconify.design"

var iconifyHTTPClient = &http.Client{Timeout: 4 * time.Second}

func FetchGroupIcon(name string) (string, string, error) {
	query := groupIconSearchQuery(name)
	if query == "" {
		return "", "", nil
	}
	iconName, err := searchIconifyIcon(query, (Group{Name: name}).IconKey())
	if err != nil {
		return "", "", err
	}
	if iconName == "" {
		return "", "", nil
	}
	svg, err := fetchIconifySVG(iconName)
	if err != nil {
		return "", "", err
	}
	return iconName, svg, nil
}

func groupIconSearchQuery(name string) string {
	key := (Group{Name: name}).IconKey()
	queries := map[string]string{
		"monitor":   "monitor",
		"code":      "code",
		"bot":       "bot",
		"wrench":    "wrench",
		"search":    "search",
		"play":      "video",
		"shopping":  "shopping-bag",
		"book":      "book-open",
		"cloud":     "cloud",
		"shield":    "shield-check",
		"database":  "database",
		"activity":  "activity",
		"mail":      "mail",
		"message":   "message-circle",
		"palette":   "palette",
		"wallet":    "wallet",
		"gamepad":   "gamepad-2",
		"newspaper": "newspaper",
		"home":      "house",
		"star":      "star",
		"grid":      "layout-grid",
		"folder":    "folder",
	}
	if query := queries[key]; query != "" {
		return query
	}
	return "folder"
}

func searchIconifyIcon(query, semanticKey string) (string, error) {
	values := url.Values{}
	values.Set("query", query)
	values.Set("prefixes", "lucide,tabler,mingcute,solar,material-symbols,mdi")
	values.Set("limit", "24")
	endpoint := iconifyAPIBase + "/search?" + values.Encode()

	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	resp, err := iconifyHTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("iconify search returned %s", resp.Status)
	}
	var payload struct {
		Icons []string `json:"icons"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64*1024)).Decode(&payload); err != nil {
		return "", err
	}
	return chooseIconifyIcon(payload.Icons, query, semanticKey), nil
}

func chooseIconifyIcon(icons []string, query, semanticKey string) string {
	if len(icons) == 0 {
		return ""
	}
	preferred := preferredIconifyIcons(semanticKey)
	for _, want := range preferred {
		for _, icon := range icons {
			if icon == want {
				return icon
			}
		}
	}
	prefixRank := map[string]int{"lucide": 0, "tabler": 1, "mingcute": 2, "solar": 3, "material-symbols": 4, "mdi": 5}
	bestIcon := icons[0]
	bestScore := 999
	for _, icon := range icons {
		prefix, iconID, ok := strings.Cut(icon, ":")
		if !ok {
			continue
		}
		score := prefixRank[prefix]*10 + 5
		if iconID == query {
			score -= 5
		} else if strings.Contains(iconID, query) {
			score -= 2
		}
		if score < bestScore {
			bestIcon = icon
			bestScore = score
		}
	}
	return bestIcon
}

func preferredIconifyIcons(semanticKey string) []string {
	return map[string][]string{
		"monitor":   {"lucide:monitor", "tabler:device-desktop"},
		"code":      {"lucide:code", "tabler:code"},
		"bot":       {"lucide:bot", "tabler:robot"},
		"wrench":    {"lucide:wrench", "tabler:tool"},
		"search":    {"lucide:search", "tabler:search"},
		"play":      {"lucide:video", "tabler:video"},
		"shopping":  {"lucide:shopping-bag", "tabler:shopping-bag"},
		"book":      {"lucide:book-open", "tabler:book"},
		"cloud":     {"lucide:cloud", "tabler:cloud"},
		"shield":    {"lucide:shield-check", "tabler:shield-check"},
		"database":  {"lucide:database", "tabler:database"},
		"activity":  {"lucide:activity", "tabler:activity"},
		"mail":      {"lucide:mail", "tabler:mail"},
		"message":   {"lucide:message-circle", "tabler:message-circle"},
		"palette":   {"lucide:palette", "tabler:palette"},
		"wallet":    {"lucide:wallet", "tabler:wallet"},
		"gamepad":   {"lucide:gamepad-2", "tabler:device-gamepad-2"},
		"newspaper": {"lucide:newspaper", "tabler:news"},
		"home":      {"lucide:house", "tabler:home"},
		"star":      {"lucide:star", "tabler:star"},
		"grid":      {"lucide:layout-grid", "tabler:layout-grid"},
		"folder":    {"lucide:folder", "tabler:folder"},
	}[semanticKey]
}

func fetchIconifySVG(iconName string) (string, error) {
	prefix, iconID, ok := strings.Cut(iconName, ":")
	if !ok || prefix == "" || iconID == "" {
		return "", fmt.Errorf("invalid icon name %q", iconName)
	}
	endpoint := fmt.Sprintf("%s/%s/%s.svg?color=%%23ffffff&width=24&height=24", iconifyAPIBase, url.PathEscape(prefix), url.PathEscape(iconID))
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	resp, err := iconifyHTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("iconify svg returned %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 32*1024))
	if err != nil {
		return "", err
	}
	svg, ok := sanitizeIconifySVG(string(body))
	if !ok {
		return "", fmt.Errorf("invalid iconify svg")
	}
	return svg, nil
}

func sanitizeIconifySVG(svg string) (string, bool) {
	svg = strings.TrimSpace(svg)
	lower := strings.ToLower(svg)
	if !strings.HasPrefix(lower, "<svg") {
		return "", false
	}
	blocked := []string{"<script", "<foreignobject", "javascript:", " onload=", " onclick=", " onerror="}
	for _, item := range blocked {
		if strings.Contains(lower, item) {
			return "", false
		}
	}
	return svg, true
}
