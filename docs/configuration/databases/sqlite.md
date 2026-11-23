# Using SQLite

By default, QingyuGist uses SQLite as the database backend.

Because SQLite is a file-based database, there is not much configuration to tweak.

The configuration `db-uri`/`QYG_DB_URI` refers to the path of the SQLite database file relative in the `$qingyugist-home/` directory (default `qingyugist.db`),
although it can be left untouched. You can also use an absolute path outside the `$qingyugist-home/` directory.

The SQLite journal mode is set to [`WAL` (Write-Ahead Logging)](https://www.sqlite.org/pragma.html#pragma_journal_mode) by default and can be changed.

#### YAML
```yaml
# default
db-uri: qingyugist.db
sqlite.journal-mode: WAL

# absolute path outside the $qingyugist-home/ directory
db-uri: file:/home/user/qingyugist.db
```

#### Environment variable
```sh
# default
QYG_DB_URI=qingyugist.db
QYG_SQLITE_JOURNAL_MODE=WAL
```

### Docker Compose
```yml
services:
  qingyugist:
    image: ghcr.io/qingyu-org/qingyugist:latest
    container_name: qingyugist
    restart: unless-stopped
    ports:
      - "6157:6157" # HTTP port
      - "2222:2222" # SSH port, can be removed if you don't use SSH
    volumes:
      - "$HOME/.qingyugist:/qingyugist"
    environment:
      QYG_SQLITE_JOURNAL_MODE: WAL
      # other configuration options
```