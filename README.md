<div align="center">

<img src="docs/public/qingyugist-mark.png?v=1.3.0" width="72" height="72" alt="QingyuGist">

<h1>QingyuGist</h1>

<p><strong>A calm, self-hosted home for your code snippets.</strong><br>
Every gist is a real Git repository: write it in the browser, or clone it and push like any other project.</p>

<p>
<a href="README.zh-CN.md">简体中文</a> ·
<a href="docs/">Documentation</a> ·
<a href="https://github.com/qingyu-org/Gist/releases/latest">Releases</a>
</p>

<p>
<img alt="CI" src="https://github.com/qingyu-org/Gist/actions/workflows/ci.yml/badge.svg?branch=master">
<img alt="Release" src="https://img.shields.io/github/v/release/qingyu-org/Gist?color=teal">
<img alt="License" src="https://img.shields.io/github/license/qingyu-org/Gist?color=blue">
<img alt="Go" src="https://img.shields.io/github/go-mod/go-version/qingyu-org/Gist">
</p>

<a href="docs/public/qingyugist-demo.png"><img src="docs/public/qingyugist-demo.png" width="900" alt="A gist with a Markdown preview"></a>

</div>

## What is this?

GitHub Gist is great, but it lives on GitHub. QingyuGist is the same idea running on your
own server: a small, quiet place to keep snippets, notes, config files and the occasional
one-file program, with a web editor for quick edits and Git underneath for everything else.

Because each gist *is* a Git repository, you are never locked in — `git clone` it, edit it
in your own editor, push it back. Revisions, forks, likes, topics and search come for free.

This project is an independent fork of [OpenGist](https://github.com/thomiceli/opengist).
It keeps the same data format, so existing installations can be upgraded in place, while
the interface, defaults and deployment have been reworked around a simpler, calmer
experience.

## Highlights

- **Write in the browser.** Markdown, CSV and Jupyter notebooks render inline, and most
  languages are highlighted, and files can be dragged straight into the editor.
- **Real Git, both ways.** Clone, pull and push over HTTPS or SSH; every save is a commit,
  and the whole revision history is browsable.
- **Share on your terms.** Public, unlisted or private gists, with likes, forks, topics and
  full-text search (Chinese word segmentation included).
- **Sign-in that fits.** Password, passkeys, TOTP, or OAuth2 with GitHub, GitLab, Gitea and
  anything speaking OpenID Connect.
- **Keep it tidy.** Upload quotas, CORS controls, optional AI moderation for public gists,
  in-app notifications and a site-wide announcement — all live from the admin panel.
- **Ship one file.** Frontend assets are embedded, so the binary and a data directory are
  the entire installation. SQLite by default, PostgreSQL and MySQL when you need them.

<details>
<summary>Dark mode</summary>
<br>
<img src="docs/public/qingyugist-demo-dark.png" width="900" alt="The same gist in dark mode">
</details>

## Quick start

### Docker

Images are published to GitHub Container Registry on every release:

```shell
docker pull ghcr.io/qingyu-org/qingyugist:latest
```

```yaml
services:
  qingyugist:
    image: ghcr.io/qingyu-org/qingyugist:latest
    container_name: qingyugist
    restart: unless-stopped
    ports:
      - "6157:6157" # web and HTTP Git
      - "2222:2222" # Git over SSH
    volumes:
      - "$HOME/.qingyugist:/qingyugist"
    # environment:
    #   UID: 1001
    #   GID: 1001
```

Then open <http://localhost:6157>. The first account you create becomes the instance
administrator.

### Prebuilt binary

Download the archive for your platform from the
[latest release](https://github.com/qingyu-org/Gist/releases/latest), then:

```shell
tar xzf qingyugist-<version>-linux-amd64.tar.gz
./qingyugist
```

The binary needs [Git](https://git-scm.com/download) 2.28 or newer, and OpenSSH if you want
SSH access. There is no configuration file: everyday options live in the admin panel, and
boot-time values are read from `QYG_*` environment variables.

### From source

Requirements: Go 1.27+, Node.js 26+, Git 2.28+ and Make.

```shell
git clone https://github.com/qingyu-org/Gist.git
cd Gist
make
./qingyugist
```

## Configuration

Everyday options live in the admin panel at `/admin/settings` and take effect immediately:
registration mode, third-party sign-in providers, upload limits, moderation, CORS, search
reindexing, branding, footer and announcement. Boot-time values are read from the environment:

| Variable | Default | Purpose |
| --- | --- | --- |
| `QYG_HOME` | `~/.qingyugist` | Data directory: repositories, database, logs, keys |
| `QYG_EXTERNAL_URL` | — | Public URL, used for OAuth callbacks and clone links |
| `QYG_DB_URI` | `qingyugist.db` | SQLite file, or a `postgres://` / `mysql://` URL |
| `QYG_HTTP_PORT` | `6157` | HTTP port; SSH is separate (`ssh.port`) |
| `QYG_SECRET_KEY` | generated | Session and MFA encryption key — back it up |

See the [configuration reference](docs/configuration/configure.md) and the
[environment cheat sheet](docs/configuration/cheat-sheet.md) for the full list.

## Upgrading from OpenGist

An existing OpenGist v1.15.x installation can be moved over without a manual database
conversion. Stop the old process, back up the data directory, and start QingyuGist on the
same data:

- The old `~/.opengist` directory, `OG_*` variables, YAML keys such as `opengist-home` and
  the legacy `/opengist` volume are still accepted.
- The SQLite database and search index are renamed in place, and the session secret, SSH
  host key, repositories, tokens and passwords are carried over untouched.
- Existing `/-/...` bookmarks redirect, and existing Git remotes keep working. Browser
  sessions end with the cookie rename, so users simply sign in again.

Two things need a manual update: the executable is now called `qingyugist` (check your
systemd unit) and the image is `ghcr.io/qingyu-org/qingyugist`. To roll back, restore the
backup or rename the database back to its previous name.

## Development

```shell
make install   # Go modules + frontend dependencies
make watch     # Vite dev server with hot reload
go test -p 1 ./internal/...
```

See the [development docs](docs/contributing/development.md) for the Vite workflow, the
database matrix and the test environment.

## Credits

QingyuGist is a fork of [OpenGist](https://github.com/thomiceli/opengist) and keeps its
data format and API compatibility. Thank you to everyone upstream who made that possible. It is built with [Go](https://go.dev), [Echo](https://echo.labstack.com),
[Vite](https://vite.dev), [Tailwind CSS](https://tailwindcss.com) and
[htmx](https://htmx.org).

## License

[AGPL-3.0](LICENSE). If you run a modified version as a network service, the license asks
you to publish those modifications.
