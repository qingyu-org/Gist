# Run QingyuGist in development mode

## With Docker

Assuming you have [Make](https://linux.die.net/man/1/make) installed,

```shell
# Clone the repository
git clone git@github.com:qingyu-org/Gist.git
cd Gist

# Build the development image
make build_dev_docker
```

Now you can run the development image with the following command:

```shell
make run_dev_docker
```

QingyuGist is now running on port 6157, you can browse http://localhost:6157

## As a binary

Requirements:
* [Git](https://git-scm.com/downloads) (2.28+)
* [Go](https://go.dev/doc/install) (1.25+)
* [Node.js](https://nodejs.org/en/download/) (20+)
* [Make](https://linux.die.net/man/1/make) (optional, but easier)

```shell
git clone git@github.com:qingyu-org/Gist.git
cd Gist
make install
make watch
```

QingyuGist is now running on port 6157, you can browse http://localhost:6157

## Translations

The interface is translated with the YAML files in `internal/i18n/locales/`,
one file per locale, named after its BCP 47 tag (`zh-CN.yml`, `pt-BR.yml`).
Copy `en-US.yml` and translate the values; the keys stay as they are.

A locale is only offered in the language menu once it covers at least 95% of
the keys in `en-US.yml`. Below that, pages would mix translated and English
sentences, so a partial file stays in the repository unused until it is
finished — nothing needs to be registered anywhere, the coverage is measured
when the server starts.

`scripts/check-translations.sh` (also run by CI) reports keys that a locale
defines but `en-US.yml` does not, which are usually typos or leftovers from a
renamed key.
