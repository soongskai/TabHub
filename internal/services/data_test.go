package services

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"tabhub/internal/db"
	"testing"
)

func newTestService(t *testing.T) *AppService {
	t.Helper()
	database, err := db.Open(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := db.Migrate(database); err != nil {
		t.Fatal(err)
	}
	return NewAppService(database)
}

func TestEnsureAdminSyncsConfiguredCredentials(t *testing.T) {
	service := newTestService(t)
	if err := service.EnsureAdmin("old-admin", "old-password"); err != nil {
		t.Fatal(err)
	}
	if err := service.EnsureAdmin("demo-user", "demo-password"); err != nil {
		t.Fatal(err)
	}

	if _, err := service.Authenticate("demo-user", "demo-password"); err != nil {
		t.Fatalf("expected configured admin credentials to work: %v", err)
	}
	if _, err := service.Authenticate("old-admin", "old-password"); err == nil {
		t.Fatal("did not expect previous admin credentials to work")
	}

	var count int
	if err := service.DB.QueryRow(`SELECT COUNT(1) FROM users`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected exactly one synced admin user, got %d", count)
	}
}

func TestEnsureAdminKeepsConfiguredUsernameWhenDuplicateExists(t *testing.T) {
	service := newTestService(t)
	if err := service.EnsureAdmin("old-admin", "old-password"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.DB.Exec(`INSERT INTO users(username, password_hash, created_at, updated_at) VALUES('demo-user', 'stale-hash', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`); err != nil {
		t.Fatal(err)
	}
	if err := service.EnsureAdmin("demo-user", "demo-password"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Authenticate("demo-user", "demo-password"); err != nil {
		t.Fatalf("expected configured admin credentials to work: %v", err)
	}
	var count int
	if err := service.DB.QueryRow(`SELECT COUNT(1) FROM users`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected exactly one configured admin user, got %d", count)
	}
}

func TestDuplicateDefaultGroupAndNullGroup(t *testing.T) {
	service := newTestService(t)
	if err := service.EnsureDefaults("https://www.baidu.com/s?wd=%s"); err != nil {
		t.Fatal(err)
	}
	var defaultID int64
	if err := service.DB.QueryRow(`SELECT id FROM groups WHERE is_system_default = 1`).Scan(&defaultID); err != nil {
		t.Fatal(err)
	}
	if err := service.CreateBookmark(&defaultID, "Demo", "https://demo.example/app/"); err != nil {
		t.Fatal(err)
	}
	err := service.CreateBookmark(nil, "Other", "https://demo.example/app")
	if !errors.Is(err, ErrDuplicateBookmark) {
		t.Fatalf("expected duplicate error, got %v", err)
	}
}

func TestEnsureDefaultsSeedsStarterGroupsAndBookmarks(t *testing.T) {
	service := newTestService(t)
	if err := service.EnsureDefaults("https://www.baidu.com/s?wd=%s"); err != nil {
		t.Fatal(err)
	}

	groups, err := service.ListGroups()
	if err != nil {
		t.Fatal(err)
	}
	groupNames := make(map[string]bool, len(groups))
	for _, group := range groups {
		groupNames[group.Name] = true
	}
	for _, name := range []string{"常用", "开发工具", "AI 工具", "影音娱乐", "购物生活"} {
		if !groupNames[name] {
			t.Fatalf("expected starter group %q, got %#v", name, groupNames)
		}
	}

	bookmarks, err := service.ListBookmarks()
	if err != nil {
		t.Fatal(err)
	}
	if len(bookmarks) != 12 {
		t.Fatalf("expected 12 starter bookmarks, got %d", len(bookmarks))
	}
	for _, bookmark := range bookmarks {
		if !strings.HasPrefix(bookmark.IconPath, "/static/default-icons/") {
			t.Fatalf("expected starter icon path for %q, got %q", bookmark.Title, bookmark.IconPath)
		}
	}
}

func TestEnsureDefaultsDoesNotSeedBookmarksWhenBookmarksExist(t *testing.T) {
	service := newTestService(t)
	if _, err := service.DB.Exec(`INSERT INTO bookmarks(title, url, sort_order, created_at, updated_at) VALUES('Existing', 'https://existing.example', 10, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`); err != nil {
		t.Fatal(err)
	}
	if err := service.EnsureDefaults("https://www.baidu.com/s?wd=%s"); err != nil {
		t.Fatal(err)
	}

	bookmarks, err := service.ListBookmarks()
	if err != nil {
		t.Fatal(err)
	}
	if len(bookmarks) != 1 || bookmarks[0].Title != "Existing" {
		t.Fatalf("expected existing bookmarks to be left alone, got %#v", bookmarks)
	}
}

func TestReorderBookmarksUpdatesSortOrder(t *testing.T) {
	service := newTestService(t)
	if _, err := service.DB.Exec(`
		INSERT INTO bookmarks(title, url, sort_order, created_at, updated_at) VALUES
		('A', 'https://a.example', 10, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
		('B', 'https://b.example', 20, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
		('C', 'https://c.example', 30, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`); err != nil {
		t.Fatal(err)
	}
	rows, err := service.DB.Query(`SELECT id FROM bookmarks ORDER BY title ASC`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if err := service.ReorderBookmarks([]int64{ids[2], ids[0], ids[1]}); err != nil {
		t.Fatal(err)
	}
	gotRows, err := service.DB.Query(`SELECT title FROM bookmarks ORDER BY sort_order ASC, id ASC`)
	if err != nil {
		t.Fatal(err)
	}
	defer gotRows.Close()
	got := make([]string, 0, 3)
	for gotRows.Next() {
		var title string
		if err := gotRows.Scan(&title); err != nil {
			t.Fatal(err)
		}
		got = append(got, title)
	}
	want := []string{"C", "A", "B"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %#v, want %#v", got, want)
		}
	}
}

func TestReorderGroupsUpdatesSortOrderIncludingDefaultGroup(t *testing.T) {
	service := newTestService(t)
	if _, err := service.DB.Exec(`
		INSERT INTO groups(name, sort_order, is_system_default, created_at, updated_at) VALUES
		('未分组', 0, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
		('A', 10, 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
		('B', 20, 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
		('C', 30, 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`); err != nil {
		t.Fatal(err)
	}
	rows, err := service.DB.Query(`SELECT id FROM groups WHERE is_system_default = 0 ORDER BY name ASC`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	var defaultID int64
	if err := service.DB.QueryRow(`SELECT id FROM groups WHERE is_system_default = 1`).Scan(&defaultID); err != nil {
		t.Fatal(err)
	}
	if err := service.ReorderGroups([]int64{ids[2], defaultID, ids[0], ids[1]}); err != nil {
		t.Fatal(err)
	}
	groups, err := service.ListGroups()
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, 0, len(groups))
	for _, group := range groups {
		got = append(got, group.Name)
	}
	want := []string{"C", "未分组", "A", "B"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %#v, want %#v", got, want)
		}
	}
	var defaultSort int
	if err := service.DB.QueryRow(`SELECT sort_order FROM groups WHERE id = ?`, defaultID).Scan(&defaultSort); err != nil {
		t.Fatal(err)
	}
	if defaultSort != 20 {
		t.Fatalf("expected default group sort_order to be updated to 20, got %d", defaultSort)
	}
}

func TestNextGroupSortOrderAppendsAfterAllGroups(t *testing.T) {
	service := newTestService(t)
	if _, err := service.DB.Exec(`
		INSERT INTO groups(name, sort_order, is_system_default, created_at, updated_at) VALUES
		('未分组', 999, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
		('A', 10, 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
		('B', 20, 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`); err != nil {
		t.Fatal(err)
	}
	sortOrder, err := service.nextGroupSortOrder()
	if err != nil {
		t.Fatal(err)
	}
	if sortOrder != 1009 {
		t.Fatalf("nextGroupSortOrder = %d, want 1009", sortOrder)
	}
}

func TestUpdateBookmarkPreservesExistingIconWithoutUpload(t *testing.T) {
	service := newTestService(t)
	if err := service.EnsureDefaults("https://www.baidu.com/s?wd=%s"); err != nil {
		t.Fatal(err)
	}
	var groupID int64
	if err := service.DB.QueryRow(`SELECT id FROM groups WHERE is_system_default = 1`).Scan(&groupID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.DB.Exec(`INSERT INTO bookmarks(group_id, title, url, icon_path, sort_order, created_at, updated_at) VALUES(NULL, 'Site', 'https://example.com', '/uploads/icons/custom.png', 10, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`); err != nil {
		t.Fatal(err)
	}
	var bookmarkID int64
	if err := service.DB.QueryRow(`SELECT id FROM bookmarks WHERE title = 'Site'`).Scan(&bookmarkID); err != nil {
		t.Fatal(err)
	}
	if err := service.UpdateBookmarkWithIcon(bookmarkID, &groupID, "Site", "https://example.com", ""); err != nil {
		t.Fatal(err)
	}
	var iconPath string
	if err := service.DB.QueryRow(`SELECT icon_path FROM bookmarks WHERE id = ?`, bookmarkID).Scan(&iconPath); err != nil {
		t.Fatal(err)
	}
	if iconPath != "/uploads/icons/custom.png" {
		t.Fatalf("expected custom icon to be preserved, got %q", iconPath)
	}
}

func TestCleanupUnusedIconsPreservesSearchEngineIcons(t *testing.T) {
	service := newTestService(t)
	dataDir := t.TempDir()
	iconDir := filepath.Join(dataDir, "uploads", "icons")
	if err := os.MkdirAll(iconDir, 0o755); err != nil {
		t.Fatal(err)
	}
	usedByBookmark := filepath.Join(iconDir, "bookmark.png")
	usedBySearch := filepath.Join(iconDir, "search.png")
	unused := filepath.Join(iconDir, "unused.png")
	for _, path := range []string{usedByBookmark, usedBySearch, unused} {
		if err := os.WriteFile(path, []byte("icon"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := service.DB.Exec(`INSERT INTO bookmarks(title, url, icon_path, sort_order, created_at, updated_at) VALUES('Site', 'https://example.com', '/uploads/icons/bookmark.png', 10, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`); err != nil {
		t.Fatal(err)
	}
	if _, err := service.DB.Exec(`INSERT INTO search_engines(name, url_template, icon_path, sort_order, created_at, updated_at) VALUES('Search', 'https://example.com?q=%s', '/uploads/icons/search.png', 10, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`); err != nil {
		t.Fatal(err)
	}
	if err := service.CleanupUnusedIcons(dataDir); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{usedByBookmark, usedBySearch} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected used icon to remain %s: %v", path, err)
		}
	}
	if _, err := os.Stat(unused); !os.IsNotExist(err) {
		t.Fatalf("expected unused icon to be removed, got %v", err)
	}
}

func TestEnsureDefaultsPreservesRenamedSystemDefault(t *testing.T) {
	service := newTestService(t)
	if _, err := service.DB.Exec(`INSERT INTO groups(name, sort_order, is_system_default, created_at, updated_at) VALUES('设备管理', 0, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`); err != nil {
		t.Fatal(err)
	}
	if err := service.EnsureDefaults("https://www.baidu.com/s?wd=%s"); err != nil {
		t.Fatal(err)
	}
	var defaultName string
	if err := service.DB.QueryRow(`SELECT name FROM groups WHERE is_system_default = 1`).Scan(&defaultName); err != nil {
		t.Fatal(err)
	}
	if defaultName != "设备管理" {
		t.Fatalf("expected renamed default group to be preserved, got %q", defaultName)
	}
}

func TestBookmarkBelongsToGroup(t *testing.T) {
	groupID := int64(1)
	bookmark := Bookmark{GroupID: &groupID}
	if !bookmark.BelongsToGroup(1) {
		t.Fatal("expected bookmark to belong to group 1")
	}
	if bookmark.BelongsToGroup(2) {
		t.Fatal("did not expect bookmark to belong to group 2")
	}
	if (Bookmark{}).BelongsToGroup(1) {
		t.Fatal("did not expect ungrouped bookmark to belong to group 1")
	}
}

func TestGroupIconKey(t *testing.T) {
	cases := map[string]string{
		"设备管理":  "monitor",
		"全栈开发":  "code",
		"AI 工具": "bot",
		"云存储":   "cloud",
		"密码安全":  "shield",
		"数据库":   "database",
		"同行店铺":  "database",
		"竞品商家":  "database",
		"店铺运营":  "database",
		"监控日志":  "activity",
		"邮箱":    "mail",
		"设计素材":  "palette",
		"常用收藏":  "star",
	}
	for name, want := range cases {
		if got := (Group{Name: name}).IconKey(); got != want {
			t.Fatalf("IconKey(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestNormalizePNGPreservesTransparentBounds(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 10, 10))
	for y := 3; y < 8; y++ {
		for x := 2; x < 7; x++ {
			src.Set(x, y, color.NRGBA{R: 37, G: 99, B: 235, A: 255})
		}
	}
	var in bytes.Buffer
	if err := png.Encode(&in, src); err != nil {
		t.Fatal(err)
	}
	out, ok := normalizePNG(in.Bytes())
	if !ok {
		t.Fatal("expected png to be normalized")
	}
	decoded, err := png.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	if got := decoded.Bounds().Size(); got.X != 10 || got.Y != 10 {
		t.Fatalf("expected transparent bounds to be preserved at 10x10, got %dx%d", got.X, got.Y)
	}
	if _, _, _, alpha := decoded.At(0, 0).RGBA(); alpha != 0 {
		t.Fatal("expected normalized png to keep transparent edge pixels")
	}
}

func TestExtractIconCandidatesHandlesFlexibleLinkAttrs(t *testing.T) {
	baseURL := mustParseURL(t, "https://example.com/app/page")
	html := `<link href='/apple.png' sizes='180x180' rel='apple-touch-icon'>
		<link type=image/png rel=icon href=icons/site.png>`

	got := extractIconCandidates(html, baseURL, http.DefaultClient)
	want := []string{"https://example.com/apple.png", "https://example.com/app/icons/site.png"}
	if len(got) != len(want) {
		t.Fatalf("expected %d candidates, got %d: %#v", len(want), len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("candidate %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestDownloadIconRejectsHTMLFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte("<html><body>login</body></html>"))
	}))
	defer server.Close()

	folder := t.TempDir()
	if got, ok := downloadIcon(server.Client(), server.URL+"/favicon.ico", folder, "example.com"); ok || got != "" {
		t.Fatalf("expected HTML favicon response to be rejected, got %q ok=%v", got, ok)
	}
}

func TestFetchFaviconUsesDeclaredPNGIcon(t *testing.T) {
	var pngIcon bytes.Buffer
	img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.NRGBA{R: 255, A: 255})
	if err := png.Encode(&pngIcon, img); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<link rel="icon" href="/logo.png">`))
		case "/logo.png":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(pngIcon.Bytes())
		default:
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte("<html>not an icon</html>"))
		}
	}))
	defer server.Close()

	path, err := FetchFavicon(server.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	if path == "" || filepath.Ext(path) != ".png" {
		t.Fatalf("expected local png icon path, got %q", path)
	}
}

func TestSaveUploadedBookmarkIcon(t *testing.T) {
	var pngIcon bytes.Buffer
	img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.NRGBA{R: 255, A: 255})
	if err := png.Encode(&pngIcon, img); err != nil {
		t.Fatal(err)
	}
	dataDir := t.TempDir()
	path, err := SaveUploadedBookmarkIcon("https://example.com", "icon.png", pngIcon.Bytes(), dataDir)
	if err != nil {
		t.Fatal(err)
	}
	if path == "" || filepath.Ext(path) != ".png" {
		t.Fatalf("expected png upload path, got %q", path)
	}
	fullPath := filepath.Join(dataDir, filepath.FromSlash(strings.TrimPrefix(path, "/")))
	if _, err := os.Stat(fullPath); err != nil {
		t.Fatalf("expected uploaded icon to exist: %v", err)
	}
}

func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

func TestRasterNeedsBackground(t *testing.T) {
	transparentLogo := image.NewNRGBA(image.Rect(0, 0, 20, 20))
	for y := 6; y < 14; y++ {
		for x := 6; x < 14; x++ {
			transparentLogo.Set(x, y, color.NRGBA{R: 37, G: 99, B: 235, A: 255})
		}
	}
	if !rasterNeedsBackground(transparentLogo) {
		t.Fatal("expected sparse transparent logo to need a background")
	}

	solidIcon := image.NewNRGBA(image.Rect(0, 0, 20, 20))
	for y := 0; y < 20; y++ {
		for x := 0; x < 20; x++ {
			solidIcon.Set(x, y, color.NRGBA{R: 37, G: 99, B: 235, A: 255})
		}
	}
	if rasterNeedsBackground(solidIcon) {
		t.Fatal("did not expect solid icon to need a background")
	}
}

func TestSVGNeedsBackground(t *testing.T) {
	withBackground := `<svg viewBox="0 0 36 36"><rect width="36" height="36" fill="#2563eb"/><path d="M1 1"/></svg>`
	if svgNeedsBackground(withBackground) {
		t.Fatal("did not expect svg with background rect to need a background")
	}
	transparentLogo := `<svg viewBox="0 0 36 36"><path d="M1 1" fill="#2563eb"/></svg>`
	if !svgNeedsBackground(transparentLogo) {
		t.Fatal("expected transparent svg logo to need a background")
	}
}
