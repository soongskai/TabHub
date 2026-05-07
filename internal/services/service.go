package services

import (
	"database/sql"
	"errors"
	"html/template"
	"net/url"
	"strings"
	"time"

	"tabhub/internal/auth"
)

type User struct {
	ID           int64
	Username     string
	PasswordHash string
}

type AppService struct {
	DB *sql.DB
}

var ErrDuplicateBookmark = errors.New("duplicate bookmark")

func NewAppService(db *sql.DB) *AppService {
	return &AppService{DB: db}
}

func (s *AppService) EnsureAdmin(username, password string) error {
	username = strings.TrimSpace(username)
	if username == "" {
		return errors.New("admin username required")
	}
	if password == "" {
		return errors.New("admin password required")
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	now := time.Now()

	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var id int64
	err = tx.QueryRow(`SELECT id FROM users WHERE username = ? ORDER BY id ASC LIMIT 1`, username).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		err = tx.QueryRow(`SELECT id FROM users ORDER BY id ASC LIMIT 1`).Scan(&id)
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if errors.Is(err, sql.ErrNoRows) {
		if _, err := tx.Exec(`INSERT INTO users(username, password_hash, created_at, updated_at) VALUES(?,?,?,?)`, username, hash, now, now); err != nil {
			return err
		}
		return tx.Commit()
	}

	if _, err := tx.Exec(`DELETE FROM users WHERE username = ? AND id <> ?`, username, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE users SET username = ?, password_hash = ?, updated_at = ? WHERE id = ?`, username, hash, now, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM users WHERE id <> ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *AppService) Authenticate(username, password string) (*User, error) {
	user := &User{}
	err := s.DB.QueryRow(`SELECT id, username, password_hash FROM users WHERE username = ?`, username).Scan(&user.ID, &user.Username, &user.PasswordHash)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("invalid credentials")
		}
		return nil, err
	}
	if err := auth.CheckPassword(user.PasswordHash, password); err != nil {
		return nil, errors.New("invalid credentials")
	}
	return user, nil
}

type Group struct {
	ID              int64         `json:"id"`
	Name            string        `json:"name"`
	IconName        string        `json:"icon_name"`
	IconSVG         template.HTML `json:"-"`
	SortOrder       int           `json:"sort_order"`
	IsSystemDefault bool          `json:"is_system_default"`
}

func (g Group) IconKey() string {
	name := strings.ToLower(strings.TrimSpace(g.Name))
	if name == "" {
		return "folder"
	}

	rules := []struct {
		icon   string
		weight int
		keys   []string
	}{
		{"database", 6, []string{"数据", "数据库", "分析", "报表", "统计", "看板", "bi", "榜单", "趋势", "指数", "指标", "采集", "爬虫", "同行", "同业", "竞品", "竞对", "对手", "运营", "销量", "流量", "转化", "市场", "排行", "排名", "监测"}},
		{"monitor", 4, []string{"设备", "device", "nas", "主机", "服务器", "server", "电脑", "硬件", "路由", "网络", "内网", "局域网", "面板", "节点"}},
		{"code", 4, []string{"开发", "代码", "code", "dev", "全栈", "编程", "git", "github", "仓库", "前端", "后端", "api", "接口", "部署", "测试", "构建"}},
		{"bot", 4, []string{"ai", "gpt", "模型", "智能", "提示词", "prompt", "大模型", "助手", "机器人"}},
		{"wrench", 3, []string{"工具", "tool", "效率", "办公", "辅助", "转换", "生成器", "计算器", "在线工具"}},
		{"search", 3, []string{"搜索", "search", "资料", "文档", "doc", "知识库", "百科", "查询", "导航"}},
		{"play", 3, []string{"影音", "视频", "音乐", "media", "movie", "tv", "影视", "电影", "直播", "播放"}},
		{"shopping", 3, []string{"购物", "shop", "买", "商城", "电商", "淘宝", "京东", "拼多多", "订单", "商品", "优惠", "店铺", "商家", "门店", "店群"}},
		{"book", 3, []string{"学习", "书", "课程", "study", "learn", "教程", "培训", "阅读", "笔记"}},
		{"cloud", 3, []string{"云", "cloud", "网盘", "存储", "对象存储", "同步", "备份", "cdn"}},
		{"shield", 3, []string{"安全", "security", "密码", "pass", "auth", "登录", "证书", "加密", "权限", "防火墙"}},
		{"activity", 3, []string{"监控", "状态", "status", "日志", "log", "告警", "运维", "健康", "性能"}},
		{"mail", 3, []string{"邮箱", "mail", "email", "邮件", "收件箱", "newsletter"}},
		{"message", 3, []string{"聊天", "chat", "消息", "社区", "论坛", "沟通", "群", "IM", "im", "客服"}},
		{"palette", 3, []string{"设计", "design", "图片", "图像", "素材", "figma", "ui", "图标", "配色", "截图", "绘图"}},
		{"wallet", 3, []string{"财务", "金融", "finance", "银行", "支付", "钱包", "money", "账单", "发票", "投资"}},
		{"gamepad", 3, []string{"游戏", "game", "娱乐", "steam", "主机游戏"}},
		{"newspaper", 3, []string{"新闻", "news", "资讯", "rss", "热点", "媒体", "文章"}},
		{"home", 3, []string{"生活", "home", "家庭", "家居", "日常"}},
		{"star", 3, []string{"收藏", "favorite", "star", "常用", "置顶", "快捷"}},
		{"grid", 3, []string{"未分组", "默认", "我的"}},
	}
	scores := map[string]int{}
	for _, rule := range rules {
		for _, key := range rule.keys {
			if strings.Contains(name, key) {
				scores[rule.icon] += rule.weight + len([]rune(key))
			}
		}
	}
	dataSignals := []string{"数据", "数据库", "分析", "报表", "统计", "看板", "bi", "指标", "指数", "采集", "趋势"}
	businessSignals := []string{"同行", "同业", "竞品", "竞对", "对手", "运营", "销量", "流量", "转化", "市场", "排行", "排名", "监测"}
	commerceTargets := []string{"店铺", "商家", "门店", "店群", "商品", "电商", "商城", "淘宝", "京东", "拼多多"}
	hasDataSignal := containsGroupIconKey(name, dataSignals)
	hasBusinessSignal := containsGroupIconKey(name, businessSignals)
	hasCommerceTarget := containsGroupIconKey(name, commerceTargets)
	if hasBusinessSignal && hasCommerceTarget {
		scores["database"] += 18
	}
	if hasDataSignal && hasCommerceTarget {
		scores["database"] += 14
	}

	bestIcon := "folder"
	bestScore := 0
	for _, rule := range rules {
		if scores[rule.icon] > bestScore {
			bestIcon = rule.icon
			bestScore = scores[rule.icon]
		}
	}
	return bestIcon
}

func containsGroupIconKey(name string, keys []string) bool {
	for _, key := range keys {
		if strings.Contains(name, key) {
			return true
		}
	}
	return false
}

type Bookmark struct {
	ID                  int64        `json:"id"`
	GroupID             *int64       `json:"group_id"`
	Title               string       `json:"title"`
	URL                 string       `json:"url"`
	IconPath            string       `json:"icon_path"`
	IconSrc             template.URL `json:"-"`
	IconNeedsBackground bool         `json:"icon_needs_background"`
	SortOrder           int          `json:"sort_order"`
}

type SearchEngine struct {
	ID                  int64        `json:"id"`
	Name                string       `json:"name"`
	URLTemplate         string       `json:"url_template"`
	IconPath            string       `json:"icon_path"`
	IconSrc             template.URL `json:"-"`
	IconNeedsBackground bool         `json:"icon_needs_background"`
	SortOrder           int          `json:"sort_order"`
	IsActive            bool         `json:"is_active"`
}

func (b Bookmark) BelongsToGroup(groupID int64) bool {
	return b.GroupID != nil && *b.GroupID == groupID
}

type Wallpaper struct {
	ID       int64  `json:"id"`
	Filename string `json:"filename"`
	Path     string `json:"path"`
	IsActive bool   `json:"is_active"`
}

func (s *AppService) EnsureDefaults(searchEngine string) error {
	var defaultID sql.NullInt64
	if err := s.DB.QueryRow(`SELECT id FROM groups WHERE is_system_default = 1 ORDER BY id LIMIT 1`).Scan(&defaultID); err != nil && err != sql.ErrNoRows {
		return err
	}
	if !defaultID.Valid {
		if _, err := s.DB.Exec(`INSERT OR IGNORE INTO groups(name, sort_order, is_system_default, created_at, updated_at) VALUES('常用', 10, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`); err != nil {
			return err
		}
		if err := s.DB.QueryRow(`SELECT id FROM groups WHERE name = '常用' ORDER BY id LIMIT 1`).Scan(&defaultID); err != nil {
			return err
		}
	}
	if _, err := s.DB.Exec(`UPDATE groups SET is_system_default = CASE WHEN id = ? THEN 1 ELSE 0 END`, defaultID.Int64); err != nil {
		return err
	}
	if _, err := s.DB.Exec(`INSERT OR IGNORE INTO settings(key, value) VALUES('search_engine', ?), ('show_time', 'true'), ('show_date', 'true'), ('overlay_opacity', '0.32')`, searchEngine); err != nil {
		return err
	}
	defaults := []struct {
		name string
		url  string
		sort int
	}{
		{searchEngineDisplayName(searchEngine), searchEngine, 0},
		{"Google", "https://www.google.com/search?q=%s", 1},
		{"必应", "https://www.bing.com/search?q=%s", 2},
	}
	for _, item := range defaults {
		iconPath := defaultSearchEngineIconPath(item.name)
		if _, err := s.DB.Exec(`INSERT OR IGNORE INTO search_engines(name, url_template, icon_path, sort_order, created_at, updated_at) VALUES(?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`, item.name, item.url, iconPath, item.sort); err != nil {
			return err
		}
	}
	return s.ensureStarterBookmarks(defaultID.Int64)
}

func defaultSearchEngineIconPath(name string) string {
	switch name {
	case "百度":
		return "/static/default-icons/baidu.svg"
	case "Google":
		return "/static/default-icons/google.svg"
	case "必应":
		return "/static/default-icons/bing.svg"
	default:
		return "/static/default-icons/search.svg"
	}
}

func (s *AppService) ensureStarterBookmarks(defaultGroupID int64) error {
	var count int
	if err := s.DB.QueryRow(`SELECT COUNT(1) FROM bookmarks`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	groups := []struct {
		name  string
		order int
	}{
		{"开发工具", 20},
		{"AI 工具", 30},
		{"影音娱乐", 40},
		{"购物生活", 50},
	}
	groupIDs := map[string]int64{"常用": defaultGroupID}
	for _, group := range groups {
		if _, err := s.DB.Exec(`INSERT OR IGNORE INTO groups(name, sort_order, is_system_default, created_at, updated_at) VALUES(?, ?, 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`, group.name, group.order); err != nil {
			return err
		}
		var id int64
		if err := s.DB.QueryRow(`SELECT id FROM groups WHERE name = ? ORDER BY id LIMIT 1`, group.name).Scan(&id); err != nil {
			return err
		}
		groupIDs[group.name] = id
	}

	bookmarks := []struct {
		group string
		title string
		url   string
		icon  string
		order int
	}{
		{"常用", "百度", "https://www.baidu.com", "baidu", 10},
		{"常用", "知乎", "https://www.zhihu.com", "zhihu", 20},
		{"常用", "微博", "https://weibo.com", "weibo", 30},
		{"开发工具", "GitHub", "https://github.com", "github", 10},
		{"开发工具", "Stack Overflow", "https://stackoverflow.com", "stackoverflow", 20},
		{"开发工具", "MDN", "https://developer.mozilla.org", "mdn", 30},
		{"AI 工具", "ChatGPT", "https://chatgpt.com", "chatgpt", 10},
		{"AI 工具", "Claude", "https://claude.ai", "claude", 20},
		{"影音娱乐", "哔哩哔哩", "https://www.bilibili.com", "bilibili", 10},
		{"影音娱乐", "YouTube", "https://www.youtube.com", "youtube", 20},
		{"购物生活", "淘宝", "https://www.taobao.com", "taobao", 10},
		{"购物生活", "京东", "https://www.jd.com", "jd", 20},
	}
	for _, bookmark := range bookmarks {
		groupID := groupIDs[bookmark.group]
		iconPath := "/static/default-icons/" + bookmark.icon + ".svg"
		if _, err := s.DB.Exec(`INSERT INTO bookmarks(group_id, title, url, icon_path, sort_order, created_at, updated_at) VALUES(?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`, groupID, bookmark.title, bookmark.url, iconPath, bookmark.order); err != nil {
			return err
		}
	}
	return nil
}

func searchEngineDisplayName(template string) string {
	parsed, err := url.Parse(strings.ReplaceAll(template, "%s", ""))
	if err != nil || parsed.Host == "" {
		return "默认搜索"
	}
	host := strings.TrimPrefix(strings.ToLower(parsed.Host), "www.")
	switch {
	case strings.Contains(host, "baidu"):
		return "百度"
	case strings.Contains(host, "google"):
		return "Google"
	case strings.Contains(host, "bing"):
		return "必应"
	default:
		return host
	}
}
