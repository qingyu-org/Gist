# Changelog

## [1.3.0](https://github.com/qingyu-org/Gist/releases/tag/v1.3.0) - 2026-11-23

### Added
- Avatars are looked up from the account's email address through **WeAvatar**
  (the default) or **Gravatar**. **Admin → Configuration → Authentication**
  chooses the provider and, optionally, a default picture for addresses the
  provider does not know. An account without an email address keeps the default
  picture, or a generated identicon when none is set
- Migration 7 moves existing accounts onto the hash of their email address and
  clears the provider CDN URLs and uploaded file names older releases stored

### Changed
- Avatar uploads are gone. The picture is managed at the provider named on the
  account settings page, which also explains that setting an email address is
  what makes an avatar appear
- An OAuth login no longer copies the provider's own picture URL. The address
  it returns is used for the lookup instead, and fills in the account's email
  when that account does not have one yet. The generic OAuth 2.0 provider no
  longer asks for an avatar field
- Collapsing the desktop sidebar now really leaves an icon rail: link labels
  and counters are visually hidden (still readable by screen readers), and the
  toggle's chevron points the way the next click moves the sidebar
- Scrollbars are thinner and translucent in both themes instead of the default
  OS chrome
- The OAuth registration page opts out of the browser's password manager and
  labels its fields as a nickname and an email, so returning from a provider no
  longer offers to save the provider password

### Fixed
- After an htmx-boosted navigation, a settings `<select>` could show both the
  native box and the styled dropdown. Native selects are now hidden inline as
  well as by class, a stale enhancement marker left behind by a restored
  snapshot is repaired, and one failing init step can no longer stop the rest
  of the page's scripts from running

### Security
- The 13 advisories fixed by upstream OpenGist 1.16.0 are included: writes to
  gists over Git HTTP without authentication, LDAP filter injection and LDAP
  usernames used unchecked as directory names, stored XSS through data
  attributes in notebooks, CPU exhaustion through `Accept-Language`,
  MFA bypass for Git over HTTP, unlisted gists discovered through `LIKE`
  wildcards or short UUIDs, private gist metadata in 404 pages, the admin
  configuration page rendered through `/assets`, expired gists still readable
  through the API, Git and SSH, sessions kept after a password change,
  private gists embedded from another origin, a stale search index after an API
  edit, and OAuth unlinking or logging out through a cross-site GET

### Breaking changes
- `GET /logout` and `GET /oauth/:provider/unlink` are gone. Logging out and
  unlinking an account require a POST request with the CSRF token; reverse
  proxies and single-sign-on logout redirects must be adjusted
- Accounts with MFA (TOTP or a passkey) must use an access token or SSH for Git
  over HTTP; their password is refused
- Short UUIDs only resolve public gists. Unlisted and private gists need their
  full UUID or custom URL
- LDAP first logins are refused when the username contains whitespace or any
  of `/ \ : ? # %`, ends with a dot, or is a reserved name

### Changed
- Changing the password logs out the other sessions
- An OAuth account can no longer be unlinked when it is the only way to log in
- Private gists cannot be embedded or fetched from another origin, and expired
  gists disappear from the API, Git, SSH and every list as soon as they expire
- The project moved to `qingyu-org/Gist`: the Go module path, binary download
  links and documentation follow it, and the container image is now
  `ghcr.io/qingyu-org/qingyugist`
- Go and JavaScript dependencies were updated

> After upgrading, run "Rebuild search index" in the admin panel so gists made
> private or unlisted through the API are removed from search.

## [1.2.1](https://github.com/qingyu-org/Gist/releases/tag/v1.2.1) - 2026-11-21

### Changed
- The language menu only lists languages that are actually finished. Every
  locale except English and Simplified Chinese was a third translated at best,
  so picking Japanese or German produced a page where a handful of words were
  localised and everything else fell back to English. Those files stay in the
  repository and appear in the menu once they cover 95% of the English keys,
  which is measured when the server starts

### Fixed
- Four locales were named with underscores (`ja_JP.yml`, `pl_PL.yml`,
  `it_IT.yml`, `uk-UK.yml`). Since the file name is the locale code, they could
  never be picked from a browser's `Accept-Language` header — and `uk-UK` is
  not a valid Ukrainian region, so they are now `ja-JP.yml`, `pl-PL.yml`,
  `it-IT.yml` and `uk-UA.yml`
- Language negotiation matches on the base language, so `zh`, `zh-Hans` or
  `en-GB` resolve to the matching language instead of falling straight through
  to English
- `<html lang>` follows the page language instead of being hardcoded to `en`,
  which helps screen readers and search engines

## [1.2.0](https://github.com/qingyu-org/Gist/releases/tag/v1.2.0) - 2026-11-20

### Added
- Gists can be commented on. The gist page shows the latest five comments with
  the composer inline, every comment has a permalink, an author can edit or
  delete their own reply, and a dedicated page lists them thirty at a time with
  Markdown preview in both places
- Comments can be answered. Threads are one level deep — a reply to a reply
  joins the same thread — and a reply notifies the author of the comment it
  answers rather than the gist owner. Deleting a thread's first comment removes
  its replies with it
- **Admin → Configuration → Comments** has a switch that hides the comment
  section, the comments page and the comment API everywhere, a second switch
  that leaves a notification in the gist owner's inbox when somebody else
  comments on their gist, and a third one that lets visitors who are not signed
  in comment under a name they type in. Guest comments can only be removed by
  the gist owner and admins. The notification links straight to the comment
- The comment API (`/api/gists/{uuid}/comments`) and the comment count returned
  by `/api/v1/gists` come from the same feature. Comments carry their
  `parent_id`, and a reply is posted by sending one
- `QYG_COMMENTS_ENABLED` turns comments off for instances that configure
  everything through the environment

### Changed
- The comment list follows this fork's interface: one card divided into rows
  instead of a stack of boxes, the owner marked with a badge, relative
  timestamps, icon buttons for edit and delete

### Fixed
- A comment is saved even when the owner notification cannot be written; the
  failure is logged instead of surfacing as an error the commenter cannot act on

## [1.1.1](https://github.com/qingyu-org/Gist/releases/tag/v1.1.1) - 2026-11-20

### Changed
- The site icon fills the corner the feather leaves empty with a `</>` mark,
  drawn with square terminals so it reads as a code symbol rather than a
  scribble at tab size. The favicon set, the documentation site's icon and the
  mark at the top of the README are all updated

## [1.1.0](https://github.com/qingyu-org/Gist/releases/tag/v1.1.0) - 2026-11-20

### Added
- The SSH host name and SSH user name shown in clone URLs are editable in
  **Admin → Configuration → Git**, so an instance behind a proxy or reachable
  through a dedicated SSH host no longer needs an environment variable
- Migration 4 adds the unique index on normalized usernames that earlier
  releases declared but never created, after checking for duplicates
- A freshly created access token is shown in a panel that stays on the page,
  with a copy button, instead of a toast that vanished after a few seconds and
  lost the only copy of the plaintext token

### Changed
- Pagination, empty states and wording across the admin panel: broadcasts are
  paginated instead of silently cut at 50, the moderation page says when
  moderation is switched off, an invitation is labelled "used up" rather than
  "expired", and revision summaries follow the interface language

### Fixed
- Indexed columns now carry explicit sizes and the database is indexed for the
  queries that run on every request: tokens by hash, gists by owner, date and
  moderation state, users by email and by each third-party account id. Existing
  databases are migrated on start
- Deleting an account left its two-factor secret behind and failed with a
  foreign-key error, so an account with 2FA enabled could not be deleted at all;
  notifications, read markers and activity rows were left orphaned as well
- `users.username` and `access_tokens.token_hash` carried a malformed GORM tag
  (a comma instead of a semicolon), so neither unique constraint nor index was
  ever created. API requests that look a token up by hash no longer scan the
  whole table
- The third-party sign-up page showed an empty provider name, LDAP could not be
  switched off once a server URL had been saved, and an OAuth-only account could
  unlink its sole sign-in method and lock itself out
- Administrators could delete their own account, and the destructive maintenance
  actions ran without asking for confirmation
- Notification pagination linked to `//notifications?page=2`
- Passkey registration ignored a failed response from the server and reported a
  bare `undefined` instead of the reason

## [1.0.4](https://github.com/qingyu-org/Gist/releases/tag/v1.0.4) - 2026-11-20

### Added
- Third-party sign-in is configured in **Admin → Configuration → Third-party
  sign-in**: one tab per provider, a switch that is off until you turn it on
  (marked with a green dot), and a display name you can set. Google and a generic
  OAuth 2.0 provider join GitHub, GitLab, Gitea and OpenID Connect, so any server
  with an authorize, token and user-info endpoint can be used
- The options that used to live only in the configuration file — OAuth clients,
  LDAP, the Meilisearch endpoint and the default search backend, HTTP git access,
  the git default branch and the API switch — are now editable in the same panel
  and take effect immediately

### Changed
- There is no configuration file anymore. Defaults are compiled in, everyday
  options live in the admin panel and boot-time values come from `QYG_*`
  environment variables. `--config` is still accepted so git hooks written by
  earlier releases keep working, but it no longer reads anything; the Helm chart
  is removed since it existed to render that file
- The site ships its own favicon — the white rounded tile from the README, as SVG
  with 32, 180 and 512px PNGs — instead of the inherited OpenGist icon, and uses
  it as the social preview image

### Fixed
- The section links in the admin configuration page navigated to the site root
  instead of scrolling within the page, because the fragment was resolved against
  the document's base URL
- A provider that is switched off can no longer be reached directly through
  `/oauth/<provider>`

## [1.0.3](https://github.com/qingyu-org/Gist/releases/tag/v1.0.3) - 2026-11-19

### Added
- AI moderation now calls the OpenAI Responses API with a strict JSON schema, so
  the verdict is structured instead of parsed out of prose. Reasoning effort,
  the maximum output tokens and the maximum document size are configurable
- A daily review quota. Gists that arrive after the cap stay pending and are
  retried every ten minutes instead of failing
- An admin review log at **Admin → Moderation**: every attempt with its verdict,
  reason, model and latency, the pending backlog, and a **Review now** action
- Site-wide notifications: admins publish a Markdown announcement from
  **Admin → Notifications**, every account sees it under the header bell and can
  mark it read. Rejections use the same inbox, so an author learns why a gist
  stayed private
- A site-wide announcement banner, rendered from Markdown in the admin settings

### Fixed
- Markdown stays readable in the light theme when the operating system prefers
  dark. The stylesheet for the light theme pulled in the automatic GitHub
  Markdown palette, which follows the OS preference, so gist previews and the
  announcement banner were painted white on white
- Moderation is no longer bypassed by `git push`, which previously published a
  gist without ever reviewing it
- Editing the files of an approved gist starts a new review; before, only a
  visibility change did, so changed content was never re-checked
- A public gist pushed over git is held private until it passes the review,
  matching the behaviour of the web editor

### Changed
- The default accent color is now teal instead of the inherited indigo, and the
  theme picker marks it when an account has not chosen one yet. The accent stays
  light on surfaces and is derived from a single CSS variable, so a personal
  choice still overrides it per account
- A bilingual (English and Simplified Chinese) README, rewritten for people who
  have never used a Git-backed pastebin, with fresh screenshots of the current
  interface
- Substantial documentation for the moderation pipeline, the review log and the
  notification inbox in the admin panel guide

## [1.0.2](https://github.com/qingyu-org/Gist/releases/tag/v1.0.2) - 2026-11-19

### Added
- Frontend type checking in CI, so TypeScript errors no longer reach a release
- A version smoke test in the release workflow, which fails before tagging when
  a binary reports the wrong version

### Changed
- Every backend form now shares a single row system: label and description on
  the left, a fixed control column aligned on the right, and number fields in a
  narrow box with the unit inside the field. The account, SSH, access token,
  invitation and gist settings pages were converted to it
- Access tokens are issued with a `qyg_` prefix; tokens issued before this
  release keep working
- Consistent card padding, table density and empty states across the admin and
  settings pages
- CI is split into lint, check, frontend, test and build jobs with concurrency
  control and timeouts. The docs and Helm workflows were removed, as the target
  repository secrets do not exist in this fork

### Fixed
- Passkey registration and login now fail closed when the stored challenge
  cannot be decoded, instead of continuing with an empty one
- Signing in through an OAuth provider now honours multi-factor
  authentication, so a linked account is no longer silently downgraded to a
  single factor
- Changing a password and disabling TOTP now require the current password, so a
  borrowed session cannot become a permanent takeover
- An invitation's expiry date was ignored and always replaced with one week
- Invitation codes and the invitation table cells came from a time-seeded
  generator; codes now use a cryptographically secure source. Table cells had
  also lost their horizontal padding
- Fork lists no longer stop after the first page
- The legacy settings endpoint rejects keys outside the registered schema
- "Bad credentials" was misspelt in the API error responses

## [1.0.1](https://github.com/qingyu-org/Gist/releases/tag/v1.0.1) - 2026-11-19

This is the first QingyuGist release: an independent hard fork focused on a
calmer interface, live administration and a fully self-contained deployment.
See the [upgrade notes](docs/update.md) before moving an existing instance.

### Added
- Live instance settings in the admin panel: registration, uploads, CORS,
  moderation, search, branding and footer links
- Admin dashboard with storage, memory, index and runtime status
- Global and per-user contribution heatmaps on the explore feeds
- Chinese search segmentation and a scheduled search reindex
- Asynchronous AI moderation for public gists
- Upload count, file size, total size and media-type limits
- Custom logo, favicon, CSS, JavaScript, sidebar links and footer records
- Light/dark themes, localized relative times and toast notifications
- Release and CI workflows publishing binaries and multi-arch GHCR images

### Changed
- Renamed the project, module, binary, environment namespace and data files to
  QingyuGist; existing data and `OG_*` variables are migrated automatically
- Replaced the `/-/` route prefix with clean top-level paths
- Rebuilt the interface around a responsive Vercel/shadcn-inspired system

### Fixed
- Theme-specific Markdown and syntax styles when switching themes at runtime
- Oversized database pool handling and Windows data-directory paths
- Several localization gaps in English and Simplified Chinese

## [1.15.3](https://github.com/qingyu-org/Gist/compare/v1.15.2...v1.15.3) - 2026-10-21
See here how to [update](https://qingyugist.io/docs/update) QingyuGist.

### Added
- Copy button on fenced code blocks in rendered Markdown (#801) (#838)
- OAuth signup through invitation links (#812)

### Fixed
- Return 413 for oversized gist forms (#805)
- Percent-encoded Unicode topic paths (#810)
- Missing list markers in rendered Markdown (#811)
- Apply database migrations inside their transaction (#817)
- Render the error page when settings fail to load (#823)
- Don't report a user's own OAuth account as linked to another user (#826)
- Render the uploaded file name as text on the create page (#830)
- Missing flash messages on plain-HTTP deployments (#834)
- User deletion (#836)

### Docs
- Add MASH playbook to community resources (#822)
- Fix typo in configuration cheat sheet (#825)

### Other
- Update Go & JS deps (#841)

## [1.15.2](https://github.com/qingyu-org/Gist/compare/v1.15.1...v1.15.2) - 2026-08-30
See here how to [update](https://qingyugist.io/docs/update) QingyuGist.

### Added
- Anonymous LDAP user search (#794)

### Fixed
- Prevent writes through symbolic links in gists (#799)
- Prevent server crashes when rendering empty CSV files
- Invalid header handling in Git `cat-file` batch reader (#802)
- Overlapping headers in narrow embeds (#803)

## [1.15.1](https://github.com/qingyu-org/Gist/compare/v1.15.0...v1.15.1) - 2026-08-04
See here how to [update](https://qingyugist.io/docs/update) QingyuGist.

### Fixed
- Admin pagination with new URLs (#782)
- Stop unbounded diff content growth for large files (#787)

## [1.15.0](https://github.com/qingyu-org/Gist/compare/v1.14.0...v1.15.0) - 2026-07-30
See here how to [update](https://qingyugist.io/docs/update) QingyuGist.

### Added
- UI rework (#757)
- Gist settings page, with metadata edition and danger zone (#757)
- Explore users page, with search and sorting (#757)
- Explore topics page (#757)

### Changed
- Move reserved routes under the `/-/` prefix: `/-/login`, `/-/register`, `/-/all`, `/-/search`, `/-/topics`, `/:user/-/liked`, ... (#757)

### Fixed
- Improve Git dumb HTTP security (#780)
- OIDC scope includes trailing whitespace if groups not configured (#775)
- Return 200 for `HEAD /` so uptime monitors don't fail (#773)
- Bind strategy configuration to deployment in Helm chart (#777)
- `QYG_HOME` env var in Dockerfile (#779)

### Other
- Update deps (#776)

## [1.14.0](https://github.com/qingyu-org/Gist/compare/v1.13.1...v1.14.0) - 2026-07-21
See here how to [update](https://qingyugist.io/docs/update) QingyuGist.

### Added
- Gist expiration + scheduled actions (#726)
- Archive gists (#739)
- Avatar upload (#730)
- Push to gists with access tokens (#752)
- Seed user/admin account via CLI (#750)
- Open Graph and Twitter Card meta tags on gist pages (#756)
- OpenSSH support (#735)
- PKCE for OAuth providers (#720)
- GitHub alerts in Markdown (#721)
- Mermaid (`.mmd`) file rendering support (#741)
- Syntax highlighting for Salesforce Apex language (#725)
- Codemirror highlighting and comment shortcut (#742)
- Confirmation prompt before deleting a file in the editor (#740)
- Default sort gists user preference (#744)
- Embedding light, dark and auto themes (#718)
- Allow copying embedded gists (#719)
- Option to disable file upload (#737)
- `HEAD` endpoint for raw and download file (#748)
- Log path config (#745)
- PostgreSQL & MySQL socket connections (#733)
- IPv6 literals support (#749)
- Allow underscores in usernames (#703)
- `rel="canonical"` link tag (#732)
- Rootless Docker image (#716)
- Korean (ko-KR) translation (#767)

### Fixed
- Improve Git handler security (#769)
- Improve git `/init` with correlation token (#751)
- Fix fatal startup error on unparseable git version (#731)
- Fix actions db (#761)
- Move users avatar directory (#760)
- Better handler for committed HTTP responses (#734)
- Sanitize DOM for ipynb rendering (#736)
- Add `no-store` header for embedded js (#724)
- Make ssh import failure a warning (#705)

### Other
- Package prebuilt frontend assets as webdist tarball (#768)
- Update deps (#762)
- Update community page with Pi share extension + gistviewer links (#755)

## [1.13.1](https://github.com/qingyu-org/Gist/compare/v1.13.0...v1.13.1) - 2026-06-10
See here how to [update](https://qingyugist.io/docs/update) QingyuGist.

### Fixed
- Embedding fix vertical scrolling and improve padding (#714)
- Fix CSS url for json embed url (#715)

## [1.13.0](https://github.com/qingyu-org/Gist/compare/v1.12.2...v1.13.0) - 2026-06-09
See here how to [update](https://qingyugist.io/docs/update) QingyuGist.

### Added
- REST API (#707) (#711) (#702)
- Limit display if there is too much files in one gist (#701)
- Topics git push option in post-receive hook (#698)
- Allow embedding Gists for a certain file only (#709)
- Arabic Translation (#706)

### Fixed
- Server SSH key generation (#708)

### Other
- Update deps Golang, JS, Docker deps (#713)
- New docs website (#710)

## [1.12.2](https://github.com/qingyu-org/Gist/compare/v1.12.1...v1.12.2) - 2026-03-14
See here how to [update](https://qingyugist.io/docs/update) QingyuGist.

### Added
- Search all fields (#622)
- Display a form to create a QingyuGist account coming from a OAuth provider (#623)
- Rebuild search index in admin options (#647)

### Fixed
- Clean file path names on file creation (#624)
- Support UTF-8 on gist download (#625)
- CSRF skipper only for GET *.js request (#627)
- Async-loaded gist embed scripts (#630)
- Make gists username/urls case insensitive in URLS (#641)
- Improve code search and index tests (#663)
- Translation strings (#659)
- Gitea avatar URL on OAuth (#674)

### [Helm Chart](helm/qingyugist)
- Add environment variables and secrets to statefulset (#644)

> Admins of QingyuGist instances may want to run "Rebuild search index" in the admin panel.

## [1.12.1](https://github.com/qingyu-org/Gist/compare/v1.12.0...v1.12.1) - 2026-02-03
See here how to [update](https://qingyugist.io/docs/update) QingyuGist.

### Added
- More translation strings (#605)

### Fixed
- Allow Access Tokens with Required Login (#611)
- Make text files renderable with mimetypes different than text/plain (#612)
- Improve security on raw files endpoint (#613)

> Admins of QingyuGist instances may want to run "Synchronize all gists previews" in the admin panel.

## [1.12.0](https://github.com/qingyu-org/Gist/compare/v1.11.1...v1.12.0) - 2026-01-27
See here how to [update](https://qingyugist.io/docs/update) QingyuGist.

### Added
- Access tokens (#602)
- Fuzzy search for gist search (#555)
- Allow Unicode letters/numbers in topics (#597)
- Resize editor height (#600)
- More translation strings (#516) (#604)

### Fixed
- Don't panic on Go TCP errors (#601)

### Other
- Reduce footprint of Docker image (#515)
- Update Go + JS deps (#603)
- Configure Dependabot for updates on Go and NPM (#449)

### [Helm Chart](helm/qingyugist)
- Use existing pvc claim of provided (#547)
- Adds StatefulSet support (#549)
- Move Prom metrics to a dedicated port + support ServiceMonitor (#599)

## [1.11.1](https://github.com/qingyu-org/Gist/compare/v1.11.0...v1.11.1) - 2025-09-30
See here how to [update](https://qingyugist.io/docs/update) QingyuGist.

### Added
- More translation strings (#511)

### Fixed
- CSV errors for rendering (#514)

### Other
- Reset default log level to warn

## [1.11.0](https://github.com/qingyu-org/Gist/compare/v1.10.0...v1.11.0) - 2025-09-21
See here how to [update](https://qingyugist.io/docs/update) QingyuGist.

### Added
- LDAP authentication (#470)
- Listen to Unix websocket (#484)
- Binary files support (#503)
- Support for rendering .ipynb Jupyter/IPython notebooks (#491)
- File upload on gist creation/edition (#507)
- Read psql sslmode from db uri (#462)
- OIDC group claim name to OpenID request (#490)
- Reworked user settings page (#467)
- Style preference tab for user (#467)
- Init gist with regular urls via git CLI (http) (#501)

### Fixed
- Gitlab avatar (#461)
- Correct German spelling, use consistent wording (#468)
- Filename unescape (#474)
- Fix Markdown preview links (#475)
- Replace Unicode characters with HTML entity codes in embed template (#480)
- Redirect to $baseUrl after auth with passkey instead of / (#482)
- Human date on iOS devices (#510)

### Docs
- Add Proxmox VE Helper-Script (#473)

### Other
- Use Helm deployment.env[] values (#471)
- Update Helm Postgres version
- Use database for Gist init queue (#498)
- Update go dep chroma (#493)

## [1.10.0](https://github.com/qingyu-org/Gist/compare/v1.9.1...v1.10.0) - 2025-04-07
See here how to [update](https://qingyugist.io/docs/update) QingyuGist.

### 🔴 Deprecations
_Removed in the next SemVer MAJOR version of QingyuGist._
* Use the configuration option `index`/`OG_INDEX` **instead of** `index.enabled`/`OG_INDEX_ENABLED`. The default value is `bleve`.
* The configuration `index.dirname`/`OG_INDEX_DIRNAME` will be removed. If you're using Bleve, the path of the index will be `qingyugist.index`.

### Added
- Helm Chart (#454)
- Meilisearch indexer (#444)
- Prometheus metrics (#439)
- Config to name the OIDC provider (#435)
- Read admin group from OIDC token claim (#445)
- More translation strings (#438)

### Fixed
- Garbled text display issues for non-English Unicode characters in browsers (#441)
- Test database when running `go test` (#442)
- Allow lag between admin invitation creation and test assertion (#452)
- gist.html using relative URL (#451)
- Do not hide file delete button on gist edit page (#447)

### Other
- Update deps Golang & JS deps (#455)

## [1.9.1](https://github.com/qingyu-org/Gist/compare/v1.9.0...v1.9.1) - 2025-02-04
See here how to [update](https://qingyugist.io/docs/update) QingyuGist.

### Added
- More translation strings (#401)

### Fixed
- SQL query for MySQL/Postgres on user profile (#424)
- User avatar on gist likes list (#425)

### Other
- Update deps Golang & JS deps (#430)

## [1.9.0](https://github.com/qingyu-org/Gist/compare/v1.8.4...v1.9.0) - 2025-02-02
See here how to [update](https://qingyugist.io/docs/update) QingyuGist.

### Added
- Topics (tags) for Gists (#413)
- Gist languages saved in database (#422)
- Search gists on user profile with title, visibility, language & topics (#422)
- Jdenticon for default avatars (#416)
- Git push option for description (#412)
- MIME type support for raw file serving (#417)

### Fixed
- Skip CSRF for embed gists (#402)
- Remove CSRF check for Git HTTP packs (#408)
- Replace path.Join with filepath.Join for file system paths (#414)
- Empty password error when trying to change the username (#418)
- Save content form on gist create error (#420)

### Other
- Refactor server code (#407)
- Remove memdb for gist init (#421)
- Added QingyuGist Docker images to Docker Hub

## [1.8.4](https://github.com/qingyu-org/Gist/compare/v1.8.3...v1.8.4) - 2024-12-15
See here how to [update](/docs/update.md) QingyuGist.

### Added
- More translation strings (#398)
- Custom instance names (#399)

### Fixed
- Prevent passwords managers autofill on filename inputs (#357)

## [1.8.3](https://github.com/qingyu-org/Gist/compare/v1.8.2...v1.8.3) - 2024-11-26
See here how to [update](/docs/update.md) QingyuGist.

### Changed
- Throw `warn` instead of `fatal` on Git global config init failure (#392)
- Define esbuild as a Javascript dependency for all other platforms (#393)

## [1.8.2](https://github.com/qingyu-org/Gist/compare/v1.8.1...v1.8.2) - 2024-11-25
See here how to [update](/docs/update.md) QingyuGist.

### Added
- More translation strings (#373) (#388)

### Changed
- Enforce git config on startup (#383)
- Respect file scheme URIs for SQLite. (#387)

### Fixed
- Convert octal notation file names in Git (#380)
- Git clone on SSH with MySQL (#382)
- Escaping for embed gists (#381)

### Other
- Update deps Golang & JS deps

## [1.8.1](https://github.com/qingyu-org/Gist/compare/v1.8.0...v1.8.1) - 2024-11-02
See here how to [update](/docs/update.md) QingyuGist.

### Changed
- Hide passkey login when login form is disabled (#369)

### Fixed
- Markdown preview (#368)
- confirm() popup messages (#370)

## [1.8.0](https://github.com/qingyu-org/Gist/compare/v1.7.5...v1.8.0) - 2024-10-31
See here how to [update](https://qingyugist.io/docs/update) QingyuGist.

### 🔴 Deprecations 
_Removed in the next SemVer MAJOR version of QingyuGist._
* Use the configuration option `db-uri`/`OG_DB_URI` **instead of** `db-filename`/`OG_DB_FILENAME`.\
  More info [here](https://qingyugist.io/docs/configuration/databases/sqlite) if you plan to keep SQLite as a DBMS for QingyuGist.

### Added
- Postgres and MySQL databases support (#335)
- Passkeys & TOTP support + MFA (#341) (#342)
- Add/Remove admins (#337)
- Queriable shorter uuids (#338)
- Use Docker secrets (#340)
- SVG preview in Markdown (#346)
- Secret key definition & move the secret key file to its parent directory (#358)
- More translation strings (#339)

### Changed
- Separate OAuth unlink URL (#336)

### Fixed
- Adding multiple empty lines in editor. (#345)
- Config URL (#343)
- Send Markdown preview data as form params (#347)
- Fix oauth endpoint to support detecting https in 'Forwarded' header, enabling google support (#359)
- Use mail handle if OAuth nickname is empty (#362)

### Other
- Use go 1.23 and update deps (#354)
- Typos in README (#363)

## [1.7.5](https://github.com/qingyu-org/Gist/compare/v1.7.4...v1.7.5) - 2024-09-12
See here how to [update](/docs/update.md) QingyuGist.

### Added
- New website for documentation using Vitepress [https://qingyugist.io](https://qingyugist.io) (#326)
- Ukrainian localization (#325)
- Dummy /metrics endpoint (#327)

## [1.7.4](https://github.com/qingyu-org/Gist/compare/v1.7.3...v1.7.4) - 2024-09-09
See here how to [update](/docs/update.md) QingyuGist.

### Added
- More translations strings (#294) (#304)
- Hide change password form when login via password disabled (#314)
- File delete button on create editor (#320)
- Assets cache header 
- Hide secret values in admin config page
- Atomic pointer for indexer (#321)

### Fixed
- Fatal error using `cases.Title()` (#313)
- Search unlisted gist (#319)

### Other
- Removed logger `trace` and `fatal` levels (#322)

## [1.7.3](https://github.com/qingyu-org/Gist/compare/v1.7.2...v1.7.3) - 2024-06-03
See here how to [update](/docs/update.md) QingyuGist.

### Added
- Setting to allow anonymous access to individual gists while still RequireLogin everywhere else (#229)
- Make edit visibility a toggle (#277)
- More translation strings (#274) (#281)
- String method to visibility (#276)

### Fixed
- Perms for http/ssh clone (#288)
- Fix translation string (#293)

### Other
- Update deps Golang & JS deps
- Check translations keys in CI (#279)
- Fix CI check for additional translations only (#289)

## [1.7.2](https://github.com/qingyu-org/Gist/compare/v1.7.1...v1.7.2) - 2024-05-05
See here how to [update](/docs/update.md) QingyuGist.

### Added
- Docs: 
  - Run with systemd as a normal user (#254)
  - Kubernetes deployment (#258)
- More translation strings (#269) (#271)

### Changed
- Rework git log parsing and truncating (#260)
- Set QingyuGist version from git tags (#261)

### Fixed
- Missing preview button when editing .md gist (#259)
- Frontend (#267)
  - Fix mermaid display 
  - Move Login/Register buttons on mobile 
  - Set minimum width on avatar

### Other
- Use go 1.22 and update deps (#244)

## [1.7.1](https://github.com/qingyu-org/Gist/compare/v1.7.0...v1.7.1) - 2024-04-05
See here how to [update](/docs/update.md) QingyuGist.

### Added
- Docs: More detailed variant for custom pages (#248)

### Fixed
- Auth page GitlabName Error (#242)
- Empty invitation on user creation (#247)

## [1.7.0](https://github.com/qingyu-org/Gist/compare/v1.6.1...v1.7.0) - 2024-04-03
See here how to [update](/docs/update.md) QingyuGist.

Note: all sessions will be invalidated after this update.

### Added
- Custom logo configuration (#209)
- Custom static links (#234)
- Invitations for closed registrations (#233)
- Set gist visibility via Git push options (#215)
- Set gist URL and title via push options (#216)
- Specify custom names in the OAuth login buttons (#214)
- Markdown preview (#224)
- Reset a user password using CLI (#226)
- Translations (#207, #210)

### Changed
- Use filesystem session store (#240)
- Move Git hook logic to QingyuGist (#213)
- Increase login for 1 year (#222)

### Fixed
- Show theme change button on responsive devices (#225)
- New line literal in embed gists (#237)

### Other
- GitHub security updates
- New docker dev env (#220)

## [1.6.1](https://github.com/qingyu-org/Gist/compare/v1.6.0...v1.6.1) - 2024-01-06
See here how to [update](/docs/update.md) QingyuGist.

### Added
- Healthcheck on Docker container (#204)
- Translations:
  - fr-FR (#201)

### Fixed
- Directory renaming on username change (#205)

## [1.6.0](https://github.com/qingyu-org/Gist/compare/v1.5.3...v1.6.0) - 2024-01-04
See here how to [update](/docs/update.md) QingyuGist.

### Added
- Embedded gists (#179)
- Gist code search (#194)
- Custom URLS for gists (#183)
- Gist JSON data/metadata (#179)
- Keep default visibility when creating a gist on the UI (#155)
- Health check endpoint (#170)
- GitLab OAuth2 login (#174)
- Syntax highlighting for more file types (#176) 
- Checkable Markdown checkboxes (#182)
- Config:
  - Log output (#172) 
  - Default git branch name (#171)
- Change username setting (#190)
- Admin actions:
  - Synchronize all gists previews (#191)
  - Reset Git server hooks for all repositories (#191)
  - Index all gists (#194)
- Translations:
  - cs-CZ (#164)
  - zh-TW (#166, #195)
  - hu-HU (#185)
  - pt-BR (#193)
- Docs (#198)

### Changed
- Updated dependencies (#197):
  - Go `1.20` -> `1.21` 
  - JavaScript packages
  - NodeJS Docker image `18` -> `20`
  - Alpine Docker image `3.17` -> `3.19`

### Fixed
- Fix reverse proxy subpath support (#192)
- Fix undecoded gist content when going back to editing in the UI (#184)
- Fix outputting non-truncated large files for editon/zip download (#184)
- Allow dashes in usernames (#184)
- Delete SSH keys associated to deleted user (#184)
- Better error message when there is no files in gist (#184)
- Show if there is no files in gist preview (#184)
- Log parsing for the 11th empty commit (#184)
- Optimize reading gist files content (#186)

## [1.5.3](https://github.com/qingyu-org/Gist/compare/v1.5.2...v1.5.3) - 2023-11-20
### Added
- es-ES translation (#139)
- Create/change account password (#156)
- Display OAuth error messages when HTTP 400 (#159)

### Fixed
- Git bare repository branch name creation (#157)
- Git file truncated output hanging (#157) 
- Home user directory detection handling (#145)
- UI changes (#158)

## [1.5.2](https://github.com/qingyu-org/Gist/compare/v1.5.1...v1.5.2) - 2023-10-16
### Added
- zh-CN translation (#130)
- ru-RU translation (#135)
- config.yml usage in the Docker container (#131)
- Longer title and description (#129)

### Fixed
- Private gist visibility (#128)
- Dark background color in Markdown rendering (#137)
- Error handling for password hashes (#132)

## [1.5.1](https://github.com/qingyu-org/Gist/compare/v1.5.0...v1.5.1) - 2023-09-29
### Added
- Hungarian translations (#123)

### Fixed
- .c and .h syntax highlighting (#119)
- Login page disabled depending on locale (#120)
- Syntax error on templates when calling locale function (#122)

## [1.5.0](https://github.com/qingyu-org/Gist/compare/v1.4.2...v1.5.0) - 2023-09-26
### Added
- Private Gist visibility (#87)
- Create gists from a special Git HTTP server remote URL (#95)
- OIDC provider integration (#98)
- Translation system (#104)
- Run `git gc` on all repositories as admin (#90)
- Unit and integration tests (#97)
- Documentation (#110, #111)
- New logo (#103)

### Changed
- Use Non-CGO SQLite instead of CGO SQLite (#100)
- Various UI changes (#84, #93)
- Improved CI/CD pipeline (#99, #113)
- Improved git http semantics and repo obfuscation (#94)
- Updated Go deps (#102)

### Fixed
- Find command for Windows users (#89)
- Retain visibility when editing a gist (#83)
- Typo on admin index page (#85)
- ViteJS dev server (#91)
- Bugs (#105)

### Breaking changes
- Removed CONFIG env var
- Removed TLS server (#101)

## [1.4.2](https://github.com/qingyu-org/Gist/compare/v1.4.1...v1.4.2) - 2023-07-17
### Added
- External url to HTML links & redirects (#75)
- Make unlisted gists not SEO crawlable (#78)
- Warning message on OAuth unlink (#79)

### Changed
- Redirect to `/all` when not logged in (#76)
- Removed Dev Docker image (#80)

## [1.4.1](https://github.com/qingyu-org/Gist/compare/v1.4.0...v1.4.1) - 2023-06-25
### ⚠️ Docker users ⚠️
QingyuGist Docker volume has been changed from `/root/.qingyugist` to `/qingyugist`, do not forget to update your
`docker-compose.yml` file or any other Docker related configuration.

Please make a backup of your QingyuGist data directory before updating.

### Fixed
- Git message remote: `warning: unable to access '/root/.config/git/attributes': Permission denied` (#71)

## [1.4.0](https://github.com/qingyu-org/Gist/compare/v1.3.0...v1.4.0) - 2023-06-23
### ⚠️ Docker users ⚠️
QingyuGist Docker volume has been changed from `/root/.qingyugist` to `/qingyugist`, do not forget to update your
`docker-compose.yml` file or any other Docker related configuration.

Please make a backup of your QingyuGist data directory before updating.

### Added
- Search gists, browse users snippets, likes and forks (#68)
- SQLite WAL journal mode by default (#54)
- Change SQLite journal mode via configuration (#54)
- Configuration via environment variables (#50)
- Docker dev image (#56)
- Choose Docker container/volumes owner via UID/GID (#63)

### Changed
- Docker volume changed from `/root/.qingyugist` to `/qingyugist` (#63)
- `DEV` environment variable renamed to `QYG_DEV` (#64)
- Use `npx` in Makefile instead of `./node_modules/.bin` (#66)
- DEPRECATED: `OG_CONFIG` environment variable (#64)

### Fixed
- Gitea URL joins (#43, #61)
- Dark mode flickering (#44)
- Typos (#42)

## [1.3.0](https://github.com/qingyu-org/Gist/compare/v1.2.0...v1.3.0) - 2023-05-27
### Added
- Disable login form via admin panel
- Syntax highlighting in Markdown code block (#29)
- Better UI for admin settings (#30)
- Disable Gravatar (#37)
- Swap between dark and light theme (#38)

### Changed
- Logs are now also appended to stdout
- Golang module name is now `github.com/qingyu-org/Gist`

### Fixed
- First account registering with OAuth is now admin
- Fix HTML entities escaping in Markdown code block (#29)

## [1.2.0](https://github.com/qingyu-org/Gist/compare/v1.1.1...v1.2.0) - 2023-05-01
### Added
- Restrict or unrestrict snippets visibility to anonymous users (#19)
- Go CI with Staticcheck

### Changed
- Filenames are now trimmed when creating a snippet (#20)
- SSH public key comments are now trimmed when adding a new key (#22)

### Fixed
- Respect ExternalUrl for OAuth (#21)
- SSH public key detection (#22)

## [1.1.1](https://github.com/qingyu-org/Gist/compare/v1.1.0...v1.1.1) - 2023-04-20
### Fixed
- Git processes are now correctly killed

## [1.1.0](https://github.com/qingyu-org/Gist/compare/v1.0.1...v1.1.0) - 2023-04-18
### Added
- GitHub and Gitea OAuth2 login
- Database migration system

### Changed
- Admin panel route from `/admin` route to `/admin-panel`
- Moved disable signup option to admin panel

### Fixed
- Truncate raw file (#4)
- Fix SSH key table constraints on user delete

## [1.0.1](https://github.com/qingyu-org/Gist/compare/v1.0.0...v1.0.1) - 2023-04-12
### Changed
- Updated base footer
- Changed redirections when not logged in

## 1.0.0 - 2023-04-10
- Initial release
