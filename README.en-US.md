# TabHub

English | [中文](README.zh-CN.md)

TabHub is a lightweight self-hosted bookmark homepage built with Go, SQLite, and vanilla frontend code. It works well as a personal server homepage, NAS dashboard, intranet start page, or browser homepage.

![Dark theme preview](docs/dark-theme.png)

## Server Requirements

TabHub runs as a single Go process with SQLite and a vanilla frontend. `docker-compose.yml` sets `GOMEMLIMIT=64MiB` by default, so 64 MB RAM is enough for light personal usage. Note that Docker image builds require more memory than runtime; on very small servers, build locally first or use a prebuilt image.

## Features

- Group management: create, edit, delete, and drag-sort sidebar groups.
- Bookmark management: add websites with title, URL, and target group.
- Icon management: fetch website favicons automatically, refresh icons manually, or upload custom PNG, JPG, GIF, SVG, WEBP, and ICO icons.
- Icon rendering: transparent icons are maximized, centered, and never cropped inside a fixed icon box.
- Bookmark sorting: drag-sort website icons inside each group.
- Search engines: add search engines, right-click to edit/delete, and click to switch the active engine.
- Wallpaper management: upload, switch, and delete wallpapers; the login page uses the current wallpaper.
- Theme switch: persistent dark and light themes.
- Settings dialog: manage theme, time/date visibility, overlay opacity, wallpapers, and logout in one place.
- Single-user mode: the admin account is synced from `config.env` on startup, and only one user is kept in the database.
- First-run defaults: an empty database is seeded with starter groups and common websites for a polished first launch.

## Screenshots

### Dark Theme

![Dark theme](docs/dark-theme.png)

### Light Theme

![Light theme](docs/light-theme.png)

### Add Group

![Add group](docs/add-group.png)

### Add Website

![Add website](docs/add-bookmark.png)

### Settings

![Settings](docs/settings.png)

## Quick Start

Run locally:

```powershell
go run ./cmd/server
```

The app reads `config.env` from the project root by default:

```env
APP_NAME=TabHub
PORT=8080
HOST_PORT=5305
ADMIN_USERNAME=admin
ADMIN_PASSWORD=admin
SESSION_SECRET=change-this-session-secret
SESSION_DAYS=60
DB_PATH=./data/app.db
DATA_DIR=./data
TZ=Asia/Shanghai
DEFAULT_SEARCH_ENGINE=https://www.baidu.com/s?wd=%s
```

Open:

```text
http://127.0.0.1:8080
```

## Docker Deployment

```powershell
docker compose --env-file config.env up -d --build
```

`PORT` is the container listen port, and `HOST_PORT` is the host access port. Data is persisted in `./data` by default.

## Data Directory

- `data/app.db`: SQLite database.
- `data/uploads/icons`: uploaded and fetched website icons.
- `data/uploads/wallpapers`: user-uploaded wallpapers.
- `web/static/default-wallpaper.jpg`: bundled default wallpaper.
- `web/static/default-icons`: bundled starter icons for first-run bookmarks.

## Support

If this project helps you, you can scan the QR code to support maintenance.

![Reward QR](docs/reward-qr.jpg)

## License

This project is licensed under the [MIT License](LICENSE).

You may use, modify, distribute, and use it commercially, but you must retain the copyright notice and license text. In practice, commercial use requires attribution.
