# Using MySQL/MariaDB

To use MySQL/MariaDB as the database backend, you need to set the database URI configuration to the connection string of your MySQL/MariaDB database with this format :

`mysql://<user>:<password>@<host>:<port>/<database>`

#### YAML
```yaml
# Example
db-uri: mysql://root:passwd@localhost:3306/qingyugist_db
```

#### Environment variable
```sh
# Example
QYG_DB_URI=mysql://root:passwd@localhost:3306/qingyugist_db
```

### Unix socket

To connect through a Unix socket instead of TCP, omit the host/port and set the `socket` query parameter to the socket path:

`mysql://<user>:<password>@/<database>?socket=<socket-path>`

```yaml
# Example
db-uri: mysql://root:passwd@/qingyugist_db?socket=/var/run/mysqld/mysqld.sock
```

### Docker Compose
```yml
services:
  qingyugist:
    image: ghcr.io/qingyu-org/qingyugist:latest
    container_name: qingyugist
    restart: unless-stopped
    depends_on:
      - mysql
    ports:
      - "6157:6157"
      - "2222:2222"
    volumes:
      - "$HOME/.qingyugist:/qingyugist"
    environment:
      QYG_DB_URI: mysql://qingyugist:secret@mysql:3306/qingyugist_db
      # other configuration options

  mysql:
    image: mysql:8.4
    restart: unless-stopped
    volumes:
      - "./qingyugist-database:/var/lib/mysql"
    environment:
      MYSQL_USER: qingyugist
      MYSQL_PASSWORD: secret
      MYSQL_DATABASE: qingyugist_db
      MYSQL_ROOT_PASSWORD: rootsecret
```