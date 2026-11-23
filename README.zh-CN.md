<div align="center">

<img src="docs/public/qingyugist-mark.png?v=1.1.1" width="72" height="72" alt="QingyuGist">

<h1>清羽 Gist</h1>

<p><strong>一个安静、自托管的代码片段之家。</strong><br>
每个 Gist 都是一个真实的 Git 仓库：可以在网页里写，也可以克隆下来像普通项目一样推送。</p>

<p>
<a href="README.md">English</a> ·
<a href="docs/">文档</a> ·
<a href="https://github.com/qingyu-org/Gist/releases/latest">版本发布</a>
</p>

<p>
<img alt="CI" src="https://github.com/qingyu-org/Gist/actions/workflows/ci.yml/badge.svg?branch=master">
<img alt="Release" src="https://img.shields.io/github/v/release/qingyu-org/Gist?color=teal">
<img alt="License" src="https://img.shields.io/github/license/qingyu-org/Gist?color=blue">
<img alt="Go" src="https://img.shields.io/github/go-mod/go-version/qingyu-org/Gist">
</p>

<a href="docs/public/qingyugist-demo-zh.png"><img src="docs/public/qingyugist-demo-zh.png" width="900" alt="带 Markdown 预览的 Gist 页面"></a>

</div>

## 这是什么？

GitHub Gist 很好用，但它住在 GitHub 上。清羽 Gist 是同一个想法，只是跑在你自己的服务器上：
一个小而安静的地方，用来存放代码片段、笔记、配置文件，偶尔也放一个小程序；随手改动用网页编辑器，
其余的一切交给 Git。

因为每个 Gist *就是* 一个 Git 仓库，你随时可以 `git clone` 下来，用自己的编辑器改完再推回去。
修订历史、派生、喜欢、主题和搜索都是顺带就有了。

本项目是 [OpenGist](https://github.com/thomiceli/opengist) 的独立分支，沿用相同的数据格式，
因此老站点可以原地升级；界面、默认值和部署方式则围绕更简洁、更安静的体验重新做过。

## 特性

- **网页里就能写。** Markdown、CSV 和 Jupyter Notebook 直接渲染，常见语言都有语法高亮，
  文件可以直接拖进编辑器。
- **双向的 Git。** 通过 HTTPS 或 SSH 克隆、拉取、推送；每次保存都是一个 commit，历史可以逐版查看。
- **按你的意愿分享。** 公开、不公开、私有三种可见性，配合喜欢、派生、主题和全文搜索（含中文分词）。
- **登录方式随你。** 密码、通行密钥、TOTP，以及 GitHub、GitLab、Gitea 和任何支持 OpenID Connect 的服务。
- **保持整洁。** 上传配额、CORS 设置、公开 Gist 的可选 AI 审核、站内通知和全站公告，都在后台即时生效。
- **一个文件就能上线。** 前端资源内嵌在二进制里，程序加一个数据目录就是完整安装；
  默认 SQLite，需要时也能换成 PostgreSQL 或 MySQL。

<details>
<summary>深色模式</summary>
<br>
<img src="docs/public/qingyugist-demo-dark.png" width="900" alt="深色模式下的同一个 Gist">
</details>

## 快速开始

### Docker

每次发布都会推送到 GitHub Container Registry：

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
      - "6157:6157" # 网页与 HTTP Git
      - "2222:2222" # SSH Git
    volumes:
      - "$HOME/.qingyugist:/qingyugist"
    # environment:
    #   UID: 1001
    #   GID: 1001
```

然后打开 <http://localhost:6157>，创建的第一个账号就是实例管理员。

### 预编译二进制

在[最新版本](https://github.com/qingyu-org/Gist/releases/latest)下载对应平台的压缩包，解压后：

```shell
tar xzf qingyugist-<version>-linux-amd64.tar.gz
./qingyugist
```

运行需要 [Git](https://git-scm.com/download) 2.28 或更高版本；要用 SSH 访问还需要 OpenSSH。
没有配置文件：日常选项都在后台，启动相关的值通过 `QYG_*` 环境变量设置。

### 从源码构建

需要 Go 1.27+、Node.js 26+、Git 2.28+ 和 Make。

```shell
git clone https://github.com/qingyu-org/Gist.git
cd Gist
make
./qingyugist
```

## 配置

日常选项都在后台 `/admin/settings`，改完立即生效：注册方式、第三方登录、上传限制、审核、CORS、搜索重建索引、
品牌与页脚公告。启动相关的值从环境变量读取：

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `QYG_HOME` | `~/.qingyugist` | 数据目录：仓库、数据库、日志、密钥 |
| `QYG_EXTERNAL_URL` | — | 对外地址，用于 OAuth 回调和克隆链接 |
| `QYG_DB_URI` | `qingyugist.db` | SQLite 文件，或 `postgres://` / `mysql://` 地址 |
| `QYG_HTTP_PORT` | `6157` | HTTP 端口；SSH 端口单独配置（`ssh.port`） |
| `QYG_SECRET_KEY` | 自动生成 | 会话与 MFA 加密密钥，请备份 |

完整的配置项见[配置说明](docs/configuration/configure.md)和
[环境变量速查表](docs/configuration/cheat-sheet.md)。

## 从 OpenGist 升级

已有的 OpenGist v1.15.x 可以直接迁移，不需要手工转换数据库。停掉旧进程、备份数据目录，
然后用同一份数据启动清羽 Gist：

- 旧的 `~/.opengist` 目录、`OG_*` 环境变量、`opengist-home` 之类的 YAML 键，
  以及旧的 `/opengist` 卷都仍然可用。
- SQLite 数据库和搜索索引会原地改名，会话密钥、SSH 主机密钥、仓库、令牌和密码保持不变。
- 旧的 `/-/...` 书签会跳转，已有的 Git 远端照常工作。浏览器会话因为 Cookie 改名会失效，重新登录即可。

有两处需要手动更新：可执行文件现在叫 `qingyugist`（注意修改 systemd 单元），镜像地址是
`ghcr.io/qingyu-org/qingyugist`。需要回退时，恢复备份或把数据库改回原来的名字。

## 开发

```shell
make install   # 拉取 Go 模块与前端依赖
make watch     # 启动带热更新的 Vite 开发服务器
go test -p 1 ./internal/...
```

Vite 工作流、数据库矩阵和测试环境见[开发文档](docs/contributing/development.md)。

## 致谢

清羽 Gist 是 [OpenGist](https://github.com/thomiceli/opengist) 的分支，沿用了它的数据格式与
API 兼容性，感谢上游的所有工作。项目使用 [Go](https://go.dev)、[Echo](https://echo.labstack.com)、
[Vite](https://vite.dev)、[Tailwind CSS](https://tailwindcss.com) 与 [htmx](https://htmx.org) 构建。

## 许可证

[AGPL-3.0](LICENSE)。如果修改后的版本以网络服务的形式提供，许可证要求公开这些修改。
