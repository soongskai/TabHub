package services

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"html/template"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var (
	linkTagPattern = regexp.MustCompile(`(?is)<link\b[^>]*>`)
	attrPattern    = regexp.MustCompile(`(?is)([a-z0-9:-]+)\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s"'>]+))`)
)

const browserUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0 Safari/537.36"
const maxIconUploadSize = 1024 * 1024

func (s *AppService) GetHomeData() ([]Group, []Bookmark, []SearchEngine, map[string]string, *Wallpaper, error) {
	groups, err := s.ListGroups()
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}
	bookmarks, err := s.ListBookmarks()
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}
	searchEngines, err := s.ListSearchEngines()
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}
	settings, err := s.GetSettings()
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}
	wallpaper, err := s.GetActiveWallpaper()
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}
	return groups, bookmarks, searchEngines, settings, wallpaper, nil
}

func (s *AppService) ListGroups() ([]Group, error) {
	rows, err := s.DB.Query(`SELECT id, name, icon_name, icon_svg, sort_order, is_system_default FROM groups ORDER BY sort_order ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []Group
	for rows.Next() {
		var item Group
		var isDefault int
		var iconSVG string
		if err := rows.Scan(&item.ID, &item.Name, &item.IconName, &iconSVG, &item.SortOrder, &isDefault); err != nil {
			return nil, err
		}
		item.IconSVG = template.HTML(iconSVG)
		item.IsSystemDefault = isDefault == 1
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *AppService) ListBookmarks() ([]Bookmark, error) {
	rows, err := s.DB.Query(`SELECT id, group_id, title, url, icon_path, sort_order FROM bookmarks ORDER BY sort_order ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []Bookmark
	for rows.Next() {
		var item Bookmark
		var groupID sql.NullInt64
		if err := rows.Scan(&item.ID, &groupID, &item.Title, &item.URL, &item.IconPath, &item.SortOrder); err != nil {
			return nil, err
		}
		if groupID.Valid {
			item.GroupID = &groupID.Int64
		}
		item.IconSrc = templateURL(item.IconPath)
		item.IconNeedsBackground = iconNeedsBackground(item.IconPath)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *AppService) ListSearchEngines() ([]SearchEngine, error) {
	settings, err := s.GetSettings()
	if err != nil {
		return nil, err
	}
	activeURL := settings["search_engine"]
	rows, err := s.DB.Query(`SELECT id, name, url_template, icon_path, sort_order FROM search_engines ORDER BY sort_order ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]SearchEngine, 0)
	for rows.Next() {
		var item SearchEngine
		if err := rows.Scan(&item.ID, &item.Name, &item.URLTemplate, &item.IconPath, &item.SortOrder); err != nil {
			return nil, err
		}
		item.IconSrc = templateURL(item.IconPath)
		item.IconNeedsBackground = iconNeedsBackground(item.IconPath)
		item.IsActive = item.URLTemplate == activeURL
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *AppService) GetSettings() (map[string]string, error) {
	rows, err := s.DB.Query(`SELECT key, value FROM settings`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	settings := map[string]string{}
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		settings[key] = value
	}
	return settings, rows.Err()
}

func (s *AppService) GetActiveWallpaper() (*Wallpaper, error) {
	item := &Wallpaper{}
	var active int
	if err := s.DB.QueryRow(`SELECT id, filename, path, is_active FROM wallpapers WHERE is_active = 1 ORDER BY id DESC LIMIT 1`).Scan(&item.ID, &item.Filename, &item.Path, &active); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	item.IsActive = active == 1
	return item, nil
}

func (s *AppService) ListWallpapers() ([]Wallpaper, error) {
	rows, err := s.DB.Query(`SELECT id, filename, path, is_active FROM wallpapers ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Wallpaper, 0)
	for rows.Next() {
		var item Wallpaper
		var active int
		if err := rows.Scan(&item.ID, &item.Filename, &item.Path, &active); err != nil {
			return nil, err
		}
		item.IsActive = active == 1
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *AppService) UpsertSetting(key, value string) error {
	_, err := s.DB.Exec(`INSERT INTO settings(key, value) VALUES(?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

func (s *AppService) CreateSearchEngine(name, rawTemplate string) error {
	name = strings.TrimSpace(name)
	cleanTemplate := normalizeSearchTemplate(rawTemplate)
	iconPath, _ := FetchFavicon(searchEngineIconURL(cleanTemplate))
	_, err := s.DB.Exec(`INSERT INTO search_engines(name, url_template, icon_path, sort_order, created_at, updated_at) VALUES(?, ?, ?, 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`, name, cleanTemplate, iconPath)
	return err
}

func (s *AppService) UpdateSearchEngine(id int64, name, rawTemplate string) error {
	name = strings.TrimSpace(name)
	cleanTemplate := normalizeSearchTemplate(rawTemplate)
	iconPath, _ := FetchFavicon(searchEngineIconURL(cleanTemplate))
	_, err := s.DB.Exec(`UPDATE search_engines SET name = ?, url_template = ?, icon_path = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, name, cleanTemplate, iconPath, id)
	return err
}

func (s *AppService) ActivateSearchEngine(id int64) error {
	var template string
	if err := s.DB.QueryRow(`SELECT url_template FROM search_engines WHERE id = ?`, id).Scan(&template); err != nil {
		return err
	}
	return s.UpsertSetting("search_engine", template)
}

func (s *AppService) DeleteSearchEngine(id int64) error {
	var activeURL string
	_ = s.DB.QueryRow(`SELECT value FROM settings WHERE key = 'search_engine'`).Scan(&activeURL)
	var deletingURL string
	if err := s.DB.QueryRow(`SELECT url_template FROM search_engines WHERE id = ?`, id).Scan(&deletingURL); err != nil {
		return err
	}
	if _, err := s.DB.Exec(`DELETE FROM search_engines WHERE id = ?`, id); err != nil {
		return err
	}
	if activeURL == deletingURL {
		var fallback string
		if err := s.DB.QueryRow(`SELECT url_template FROM search_engines ORDER BY sort_order ASC, id ASC LIMIT 1`).Scan(&fallback); err == nil {
			return s.UpsertSetting("search_engine", fallback)
		}
	}
	return nil
}

func (s *AppService) CreateGroup(name string) error {
	name = strings.TrimSpace(name)
	iconName, iconSVG, _ := FetchGroupIcon(name)
	sortOrder, err := s.nextGroupSortOrder()
	if err != nil {
		return err
	}
	_, err = s.DB.Exec(`INSERT INTO groups(name, icon_name, icon_svg, sort_order, is_system_default, created_at, updated_at) VALUES(?, ?, ?, ?, 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`, name, iconName, iconSVG, sortOrder)
	return err
}

func (s *AppService) UpdateGroup(id int64, name string) error {
	name = strings.TrimSpace(name)
	iconName, iconSVG, _ := FetchGroupIcon(name)
	_, err := s.DB.Exec(`UPDATE groups SET name = ?, icon_name = ?, icon_svg = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, name, iconName, iconSVG, id)
	return err
}

func (s *AppService) RefreshMissingGroupIcons() error {
	rows, err := s.DB.Query(`SELECT id, name FROM groups WHERE icon_svg = '' ORDER BY id ASC`)
	if err != nil {
		return err
	}
	defer rows.Close()
	type groupRow struct {
		id   int64
		name string
	}
	items := make([]groupRow, 0)
	for rows.Next() {
		var item groupRow
		if err := rows.Scan(&item.id, &item.name); err != nil {
			return err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, item := range items {
		iconName, iconSVG, err := FetchGroupIcon(item.name)
		if err != nil || iconSVG == "" {
			continue
		}
		if _, err := s.DB.Exec(`UPDATE groups SET icon_name = ?, icon_svg = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, iconName, iconSVG, item.id); err != nil {
			return err
		}
	}
	return nil
}

func (s *AppService) DeleteGroup(id int64) error {
	var ungroupedID int64
	if err := s.DB.QueryRow(`SELECT id FROM groups WHERE is_system_default = 1 LIMIT 1`).Scan(&ungroupedID); err != nil {
		return err
	}
	if _, err := s.DB.Exec(`UPDATE bookmarks SET group_id = ? WHERE group_id = ?`, ungroupedID, id); err != nil {
		return err
	}
	_, err := s.DB.Exec(`DELETE FROM groups WHERE id = ? AND is_system_default = 0`, id)
	return err
}

func (s *AppService) CreateBookmark(groupID *int64, title, rawURL string) error {
	return s.CreateBookmarkWithIcon(groupID, title, rawURL, "")
}

func (s *AppService) CreateBookmarkWithIcon(groupID *int64, title, rawURL, uploadedIconPath string) error {
	cleanURL := NormalizeURL(rawURL)
	cleanTitle := strings.TrimSpace(title)
	exists, err := s.bookmarkDuplicateExists(0, groupID, cleanTitle, cleanURL)
	if err != nil {
		return err
	}
	if exists {
		return ErrDuplicateBookmark
	}
	iconPath := strings.TrimSpace(uploadedIconPath)
	if iconPath == "" {
		iconPath, _ = FetchFavicon(cleanURL)
	}
	sortOrder, err := s.nextBookmarkSortOrder(groupID)
	if err != nil {
		return err
	}
	_, err = s.DB.Exec(`INSERT INTO bookmarks(group_id, title, url, icon_path, sort_order, created_at, updated_at) VALUES(?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`, groupID, cleanTitle, cleanURL, iconPath, sortOrder)
	return err
}

func (s *AppService) UpdateBookmark(id int64, groupID *int64, title, rawURL string) error {
	return s.UpdateBookmarkWithIcon(id, groupID, title, rawURL, "")
}

func (s *AppService) UpdateBookmarkWithIcon(id int64, groupID *int64, title, rawURL, uploadedIconPath string) error {
	cleanURL := NormalizeURL(rawURL)
	cleanTitle := strings.TrimSpace(title)
	exists, err := s.bookmarkDuplicateExists(id, groupID, cleanTitle, cleanURL)
	if err != nil {
		return err
	}
	if exists {
		return ErrDuplicateBookmark
	}
	iconPath := strings.TrimSpace(uploadedIconPath)
	if iconPath == "" {
		if err := s.DB.QueryRow(`SELECT icon_path FROM bookmarks WHERE id = ?`, id).Scan(&iconPath); err != nil {
			return err
		}
	}
	_, err = s.DB.Exec(`UPDATE bookmarks SET group_id = ?, title = ?, url = ?, icon_path = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, groupID, cleanTitle, cleanURL, iconPath, id)
	return err
}

func (s *AppService) bookmarkDuplicateExists(excludeID int64, groupID *int64, title, cleanURL string) (bool, error) {
	groupCondition := "group_id IS NULL"
	args := []any{excludeID}

	defaultGroupID, hasDefaultGroup, err := s.systemDefaultGroupID()
	if err != nil {
		return false, err
	}
	if groupID != nil {
		if hasDefaultGroup && *groupID == defaultGroupID {
			groupCondition = "(group_id IS NULL OR group_id = ?)"
			args = append(args, defaultGroupID)
		} else {
			groupCondition = "group_id = ?"
			args = append(args, *groupID)
		}
	} else if hasDefaultGroup {
		groupCondition = "(group_id IS NULL OR group_id = ?)"
		args = append(args, defaultGroupID)
	}
	args = append(args, title, cleanURL)

	var count int
	err = s.DB.QueryRow(fmt.Sprintf(`
		SELECT COUNT(1)
		FROM bookmarks
		WHERE id <> ?
		  AND %s
		  AND (
		    title = ?
		    OR LOWER(RTRIM(url, '/')) = LOWER(RTRIM(?, '/'))
		  )
	`, groupCondition), args...).Scan(&count)
	return count > 0, err
}

func (s *AppService) systemDefaultGroupID() (int64, bool, error) {
	var id int64
	err := s.DB.QueryRow(`SELECT id FROM groups WHERE is_system_default = 1 LIMIT 1`).Scan(&id)
	if err == sql.ErrNoRows {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return id, true, nil
}

func (s *AppService) DeleteBookmark(id int64) error {
	_, err := s.DB.Exec(`DELETE FROM bookmarks WHERE id = ?`, id)
	return err
}

func (s *AppService) DeleteBookmarksByURL(rawURL string) error {
	_, err := s.DB.Exec(`DELETE FROM bookmarks WHERE url = ?`, strings.TrimSpace(rawURL))
	return err
}

func (s *AppService) ReorderBookmarks(ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for index, id := range ids {
		if _, err := tx.Exec(`UPDATE bookmarks SET sort_order = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, (index+1)*10, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *AppService) ReorderGroups(ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for index, id := range ids {
		if _, err := tx.Exec(`UPDATE groups SET sort_order = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, (index+1)*10, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *AppService) nextGroupSortOrder() (int, error) {
	var sortOrder int
	if err := s.DB.QueryRow(`SELECT COALESCE(MAX(sort_order), 0) + 10 FROM groups`).Scan(&sortOrder); err != nil {
		return 0, err
	}
	return sortOrder, nil
}

func (s *AppService) nextBookmarkSortOrder(groupID *int64) (int, error) {
	query := `SELECT COALESCE(MAX(sort_order), 0) + 10 FROM bookmarks WHERE group_id IS NULL`
	args := []any{}
	if groupID != nil {
		query = `SELECT COALESCE(MAX(sort_order), 0) + 10 FROM bookmarks WHERE group_id = ?`
		args = append(args, *groupID)
	}
	var sortOrder int
	if err := s.DB.QueryRow(query, args...).Scan(&sortOrder); err != nil {
		return 0, err
	}
	return sortOrder, nil
}

func (s *AppService) CleanupUnusedIcons(dataDir string) error {
	rows, err := s.DB.Query(`
		SELECT DISTINCT icon_path FROM bookmarks WHERE icon_path <> ''
		UNION
		SELECT DISTINCT icon_path FROM search_engines WHERE icon_path <> ''
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	used := make(map[string]struct{})
	for rows.Next() {
		var iconPath string
		if err := rows.Scan(&iconPath); err != nil {
			return err
		}
		if strings.HasPrefix(iconPath, "/uploads/icons/") {
			used[filepath.Base(iconPath)] = struct{}{}
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	iconDir := filepath.Join(dataDir, "uploads", "icons")
	entries, err := os.ReadDir(iconDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if _, ok := used[entry.Name()]; ok {
			continue
		}
		_ = os.Remove(filepath.Join(iconDir, entry.Name()))
	}
	return nil
}

func (s *AppService) MigrateBookmarkIconPaths() error {
	rows, err := s.DB.Query(`SELECT id, url FROM bookmarks WHERE url <> ''`)
	if err != nil {
		return err
	}
	defer rows.Close()

	type item struct {
		id  int64
		url string
	}
	items := make([]item, 0)
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.id, &it.url); err != nil {
			return err
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, it := range items {
		iconPath, err := FetchFavicon(it.url)
		if err != nil || iconPath == "" {
			continue
		}
		if _, err := s.DB.Exec(`UPDATE bookmarks SET icon_path = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, iconPath, it.id); err != nil {
			return err
		}
	}
	return nil
}

func (s *AppService) RefreshAllBookmarkIcons() error {
	rows, err := s.DB.Query(`SELECT id FROM bookmarks ORDER BY id ASC`)
	if err != nil {
		return err
	}
	defer rows.Close()

	ids := make([]int64, 0)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, id := range ids {
		if err := s.RefreshBookmarkIcon(id); err != nil {
			continue
		}
	}
	return nil
}

func (s *AppService) RefreshBookmarkIcon(id int64) error {
	var rawURL string
	if err := s.DB.QueryRow(`SELECT url FROM bookmarks WHERE id = ?`, id).Scan(&rawURL); err != nil {
		return err
	}
	iconPath, err := FetchFavicon(rawURL)
	if err != nil {
		return err
	}
	_, err = s.DB.Exec(`UPDATE bookmarks SET icon_path = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, iconPath, id)
	return err
}

func (s *AppService) SaveWallpaper(filename string, content []byte, dataDir string) error {
	folder := filepath.Join(dataDir, "uploads", "wallpapers")
	if err := os.MkdirAll(folder, 0o755); err != nil {
		return err
	}
	cleanName := fmt.Sprintf("%d-%s", time.Now().UnixNano(), filepath.Base(filename))
	fullPath := filepath.Join(folder, cleanName)
	if err := os.WriteFile(fullPath, content, 0o644); err != nil {
		return err
	}
	if _, err := s.DB.Exec(`UPDATE wallpapers SET is_active = 0`); err != nil {
		return err
	}
	_, err := s.DB.Exec(`INSERT INTO wallpapers(filename, path, is_active, created_at) VALUES(?, ?, 1, CURRENT_TIMESTAMP)`, filename, filepath.ToSlash(filepath.Join("uploads", "wallpapers", cleanName)))
	return err
}

func SaveUploadedBookmarkIcon(rawURL, filename string, content []byte, dataDir string) (string, error) {
	if len(content) == 0 {
		return "", nil
	}
	if len(content) > maxIconUploadSize {
		return "", fmt.Errorf("图标不能超过 1MB")
	}
	cleanURL := NormalizeURL(rawURL)
	parsed, err := url.Parse(cleanURL)
	if err != nil || parsed.Host == "" {
		return "", fmt.Errorf("invalid url")
	}
	ext := strings.ToLower(filepath.Ext(filename))
	if ext == "" || len(ext) > 5 {
		ext = ".ico"
	}
	ext = iconExtension("", content, ext)
	if ext == "" {
		return "", fmt.Errorf("请上传 PNG、JPG、GIF、SVG、WEBP 或 ICO 图标")
	}
	if normalized, ok := normalizePNG(content); ok {
		content = normalized
		ext = ".png"
	}
	folder := filepath.Join(dataDir, "uploads", "icons")
	if err := os.MkdirAll(folder, 0o755); err != nil {
		return "", err
	}
	hostKey := strings.NewReplacer(":", "_", "/", "_", "\\", "_").Replace(parsed.Host)
	cleanName := fmt.Sprintf("%s-%d%s", hostKey, time.Now().UnixNano(), ext)
	fullPath := filepath.Join(folder, cleanName)
	if err := os.WriteFile(fullPath, content, 0o644); err != nil {
		return "", err
	}
	return "/uploads/icons/" + cleanName, nil
}

func (s *AppService) SetActiveWallpaper(id int64) error {
	if _, err := s.DB.Exec(`UPDATE wallpapers SET is_active = 0`); err != nil {
		return err
	}
	_, err := s.DB.Exec(`UPDATE wallpapers SET is_active = 1 WHERE id = ?`, id)
	return err
}

func (s *AppService) DeleteWallpaper(id int64, dataDir string) error {
	var path string
	if err := s.DB.QueryRow(`SELECT path FROM wallpapers WHERE id = ?`, id).Scan(&path); err != nil {
		return err
	}
	_ = os.Remove(filepath.Join(dataDir, filepath.FromSlash(path)))
	_, err := s.DB.Exec(`DELETE FROM wallpapers WHERE id = ?`, id)
	return err
}

func NormalizeURL(rawURL string) string {
	cleanURL := strings.TrimSpace(rawURL)
	if cleanURL == "" {
		return cleanURL
	}
	parsed, err := url.Parse(cleanURL)
	if err == nil && parsed.Scheme != "" {
		return cleanURL
	}
	return "https://" + strings.TrimLeft(cleanURL, "/")
}

func normalizeSearchTemplate(rawTemplate string) string {
	clean := strings.TrimSpace(rawTemplate)
	if clean == "" {
		return clean
	}
	if !strings.Contains(clean, "%s") {
		separator := "?"
		if strings.Contains(clean, "?") {
			separator = "&"
		}
		clean += separator + "q=%s"
	}
	return NormalizeURL(clean)
}

func searchEngineIconURL(template string) string {
	replaced := strings.ReplaceAll(template, "%s", "")
	parsed, err := url.Parse(replaced)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return template
	}
	return parsed.Scheme + "://" + parsed.Host + "/"
}

func FetchFavicon(rawURL string) (string, error) {
	parsed, err := url.Parse(NormalizeURL(rawURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("invalid url")
	}
	client := &http.Client{Timeout: 4 * time.Second}
	candidates := []string{parsed.Scheme + "://" + parsed.Host + "/favicon.ico"}
	resp, err := httpGet(client, parsed.String(), "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	if err == nil {
		defer resp.Body.Close()
		buf, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
		pageURL := resp.Request.URL
		candidates = append(extractIconCandidates(string(buf), pageURL, client), candidates...)
	}

	folder := filepath.Join("data", "uploads", "icons")
	if err := os.MkdirAll(folder, 0o755); err != nil {
		return "", nil
	}

	hostKey := strings.NewReplacer(":", "_", "/", "_", "\\", "_").Replace(parsed.Host)
	for _, iconURL := range uniqueStrings(candidates) {
		iconPath, ok := downloadIcon(client, iconURL, folder, hostKey)
		if ok {
			return iconPath, nil
		}
	}

	return "", nil
}

func extractIconCandidates(html string, baseURL *url.URL, client *http.Client) []string {
	candidates := make([]string, 0)
	manifests := make([]string, 0)
	for _, tag := range linkTagPattern.FindAllString(html, -1) {
		attrs := parseHTMLAttrs(tag)
		rel := strings.ToLower(attrs["rel"])
		href := strings.TrimSpace(attrs["href"])
		if href == "" {
			continue
		}
		if strings.Contains(rel, "icon") {
			if resolved := resolveURL(baseURL, href); resolved != "" {
				candidates = append(candidates, resolved)
			}
			continue
		}
		if strings.Contains(rel, "manifest") {
			if resolved := resolveURL(baseURL, href); resolved != "" {
				manifests = append(manifests, resolved)
			}
		}
	}
	for _, manifestURL := range manifests {
		candidates = append(candidates, extractManifestIconCandidates(manifestURL, client)...)
	}
	return candidates
}

func parseHTMLAttrs(tag string) map[string]string {
	attrs := map[string]string{}
	for _, match := range attrPattern.FindAllStringSubmatch(tag, -1) {
		value := match[2]
		if value == "" {
			value = match[3]
		}
		if value == "" {
			value = match[4]
		}
		attrs[strings.ToLower(match[1])] = strings.TrimSpace(value)
	}
	return attrs
}

func resolveURL(baseURL *url.URL, raw string) string {
	if baseURL == nil || raw == "" {
		return ""
	}
	resolved, err := baseURL.Parse(raw)
	if err != nil || resolved.Scheme == "" || resolved.Host == "" {
		return ""
	}
	return resolved.String()
}

func extractManifestIconCandidates(manifestURL string, client *http.Client) []string {
	resp, err := httpGet(client, manifestURL, "application/manifest+json, application/json;q=0.9, */*;q=0.1")
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return nil
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 128*1024))
	if err != nil {
		return nil
	}
	type manifestIcon struct {
		Src string `json:"src"`
	}
	var manifest struct {
		Icons []manifestIcon `json:"icons"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return nil
	}
	baseURL := resp.Request.URL
	candidates := make([]string, 0, len(manifest.Icons))
	for _, icon := range manifest.Icons {
		if resolved := resolveURL(baseURL, icon.Src); resolved != "" {
			candidates = append(candidates, resolved)
		}
	}
	return candidates
}

func uniqueStrings(items []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(items))
	for _, item := range items {
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func downloadIcon(client *http.Client, iconURL, folder, hostKey string) (string, bool) {
	iconResp, err := httpGet(client, iconURL, "image/avif,image/webp,image/apng,image/svg+xml,image/*,*/*;q=0.8")
	if err != nil {
		return "", false
	}
	defer iconResp.Body.Close()
	if iconResp.StatusCode < 200 || iconResp.StatusCode >= 400 {
		return "", false
	}
	if iconResp.ContentLength > 1024*1024 {
		return "", false
	}

	ext := ".ico"
	if parsedIconURL, err := url.Parse(iconURL); err == nil {
		ext = path.Ext(parsedIconURL.Path)
	}
	if ext == "" || len(ext) > 5 {
		ext = ".ico"
	}
	content, err := io.ReadAll(io.LimitReader(iconResp.Body, 1024*1024))
	if err != nil {
		return "", false
	}
	ext = iconExtension(iconResp.Header.Get("Content-Type"), content, ext)
	if ext == "" {
		return "", false
	}
	if normalized, ok := normalizePNG(content); ok {
		content = normalized
		ext = ".png"
	}
	filename := hostKey + ext
	fullPath := filepath.Join(folder, filename)
	if err := os.WriteFile(fullPath, content, 0o644); err != nil {
		return "", false
	}
	return "/uploads/icons/" + filename, true
}

func httpGet(client *http.Client, rawURL, accept string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", browserUserAgent)
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	return client.Do(req)
}

func iconExtension(contentType string, content []byte, fallback string) string {
	if mimeType, _, err := mime.ParseMediaType(contentType); err == nil {
		switch strings.ToLower(mimeType) {
		case "image/x-icon", "image/vnd.microsoft.icon":
			return ".ico"
		case "image/png":
			return ".png"
		case "image/jpeg":
			return ".jpg"
		case "image/gif":
			return ".gif"
		case "image/svg+xml":
			return ".svg"
		case "image/webp":
			return ".webp"
		case "text/html", "application/xhtml+xml":
			return ""
		}
	}
	return sniffIconExtension(content, fallback)
}

func sniffIconExtension(content []byte, fallback string) string {
	trimmed := bytes.TrimSpace(content)
	lowerTrimmed := bytes.ToLower(trimmed[:min(len(trimmed), 256)])
	switch {
	case len(content) >= 4 && content[0] == 0x00 && content[1] == 0x00 && (content[2] == 0x01 || content[2] == 0x02) && content[3] == 0x00:
		return ".ico"
	case bytes.HasPrefix(content, []byte{0x89, 'P', 'N', 'G'}):
		return ".png"
	case bytes.HasPrefix(content, []byte{0xff, 0xd8, 0xff}):
		return ".jpg"
	case bytes.HasPrefix(content, []byte("GIF87a")) || bytes.HasPrefix(content, []byte("GIF89a")):
		return ".gif"
	case len(content) >= 12 && bytes.Equal(content[:4], []byte("RIFF")) && bytes.Equal(content[8:12], []byte("WEBP")):
		return ".webp"
	case bytes.HasPrefix(lowerTrimmed, []byte("<svg")) || bytes.Contains(lowerTrimmed, []byte("<svg")):
		return ".svg"
	case bytes.HasPrefix(lowerTrimmed, []byte("<!doctype html")) || bytes.HasPrefix(lowerTrimmed, []byte("<html")):
		return ""
	}
	if fallback == ".ico" || fallback == ".png" || fallback == ".jpg" || fallback == ".jpeg" || fallback == ".gif" || fallback == ".svg" || fallback == ".webp" {
		return fallback
	}
	return ""
}

func normalizePNG(content []byte) ([]byte, bool) {
	img, err := png.Decode(bytes.NewReader(content))
	if err != nil {
		return nil, false
	}
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		return nil, false
	}
	return out.Bytes(), true
}

func iconNeedsBackground(iconPath string) bool {
	if iconPath == "" {
		return false
	}
	if strings.HasPrefix(iconPath, "data:image/svg+xml") {
		return svgNeedsBackground(iconPath)
	}
	if strings.HasPrefix(iconPath, "/uploads/") {
		return localIconNeedsBackground(filepath.Join("data", "uploads", filepath.FromSlash(strings.TrimPrefix(iconPath, "/uploads/"))))
	}
	return false
}

func localIconNeedsBackground(fullPath string) bool {
	ext := strings.ToLower(filepath.Ext(fullPath))
	switch ext {
	case ".svg":
		content, err := os.ReadFile(fullPath)
		if err != nil {
			return false
		}
		return svgNeedsBackground(string(content))
	case ".png", ".jpg", ".jpeg", ".gif":
		file, err := os.Open(fullPath)
		if err != nil {
			return false
		}
		defer file.Close()
		img, _, err := image.Decode(file)
		if err != nil {
			return false
		}
		return rasterNeedsBackground(img)
	default:
		return false
	}
}

func rasterNeedsBackground(img image.Image) bool {
	bounds := img.Bounds()
	if bounds.Empty() {
		return false
	}
	total := bounds.Dx() * bounds.Dy()
	transparent := 0
	edgeTotal := 0
	edgeOpaque := 0
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			_, _, _, alpha := img.At(x, y).RGBA()
			if alpha < 0xf000 {
				transparent++
			}
			if x == bounds.Min.X || y == bounds.Min.Y || x == bounds.Max.X-1 || y == bounds.Max.Y-1 {
				edgeTotal++
				if alpha >= 0xf000 {
					edgeOpaque++
				}
			}
		}
	}
	transparentRatio := float64(transparent) / float64(total)
	edgeOpaqueRatio := float64(edgeOpaque) / float64(edgeTotal)
	return transparentRatio > 0.15 && edgeOpaqueRatio < 0.15
}

func svgNeedsBackground(raw string) bool {
	if strings.HasPrefix(raw, "data:image/svg+xml") {
		if comma := strings.Index(raw, ","); comma >= 0 {
			decoded, err := url.QueryUnescape(raw[comma+1:])
			if err == nil {
				raw = decoded
			}
		}
	}
	normalized := strings.ToLower(raw)
	for _, match := range regexp.MustCompile(`(?s)<rect\b[^>]*>`).FindAllString(normalized, -1) {
		hasFullSize := strings.Contains(match, `width="100%"`) ||
			strings.Contains(match, `width='100%'`) ||
			strings.Contains(match, `width="36"`) ||
			strings.Contains(match, `width='36'`) ||
			strings.Contains(match, `width="256"`) ||
			strings.Contains(match, `width='256'`)
		hasVisibleFill := strings.Contains(match, "fill=") &&
			!strings.Contains(match, `fill="none"`) &&
			!strings.Contains(match, `fill='none'`) &&
			!strings.Contains(match, `fill="transparent"`) &&
			!strings.Contains(match, `fill='transparent'`)
		if hasFullSize && hasVisibleFill {
			return false
		}
	}
	return true
}

func templateURL(raw string) template.URL {
	return template.URL(raw)
}
