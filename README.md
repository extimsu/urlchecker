# urlchecker

The tiny cli-tool for health-checking urls

## Getting started

This project requires Go to be installed. On OS X with Homebrew you can just run `brew install go`.

build:

```console
make build
```

run:

```console
./bin/urlchecker --url extim.su
```

You can also check multiple urls in the same:

```console
./bin/urlchecker --url extim.su,google.com:80,example.com:443
```

### Docker

```bash
docker run docker.io/extim/urlchecker:0.1.0 --url extim.su
OR
docker run docker.io/extim/urlchecker:0.1.0 --url extim.su --port 443
```
