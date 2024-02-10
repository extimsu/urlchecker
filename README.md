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

Yu can specify protocol (--protocol). It's can be tcp or udp.

```console
./bin/urlchecker --url google.com:53 --protocol udp
```

### Docker

```console
docker run docker.io/extim/urlchecker --url extim.su
```

```console
docker run docker.io/extim/urlchecker --url extim.su,google.com:80,example.com:443
```

```console
docker run docker.io/extim/urlchecker --url extim.su:80,google.com:80,example.com:443 --port 443
```

```console
docker run docker.io/extim/urlchecker --url google.com:53 --protocol udp
```
