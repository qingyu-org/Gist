# Using PostgreSQL

To use PostgreSQL as the database backend, you need to set the database URI configuration to the connection string of your PostgreSQL database with this format :

`postgres://<user>:<password>@<host>:<port>/<database>`

#### YAML
```yaml
# Example
db-uri: postgres://postgres:passwd@localhost:5432/qingyugist_db
```

#### Environment variable
```sh
# Example
QYG_DB_URI=postgres://postgres:passwd@localhost:5432/qingyugist_db
```

### Unix socket

To connect through a Unix socket instead of TCP, omit the host/port and set the `socket` query parameter to the directory containing the socket:

`postgres://<user>:<password>@/<database>?socket=<socket-directory>`

```yaml
# Example
db-uri: postgres://postgres:passwd@/qingyugist_db?socket=/var/run/postgresql
```

### Docker Compose
```yml
services:
  qingyugist:
    image: ghcr.io/qingyu-org/qingyugist:latest
    container_name: qingyugist
    restart: unless-stopped
    depends_on:
      - postgres
    ports:
      - "6157:6157"
      - "2222:2222"
    volumes:
      - "$HOME/.qingyugist:/qingyugist"
    environment:
      QYG_DB_URI: postgres://qingyugist:secret@postgres:5432/qingyugist_db
      # other configuration options

  postgres:
    image: postgres:16.4
    restart: unless-stopped
    volumes:
      - "./qingyugist-database:/var/lib/postgresql/data"
    environment:
      POSTGRES_USER: qingyugist
      POSTGRES_PASSWORD: secret
      POSTGRES_DB: qingyugist_db
```