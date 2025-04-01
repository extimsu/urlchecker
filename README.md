# urlchecker

The tiny cli-tool for health-checking urls

## Getting started

This project requires Go to be installed. On OS X with Homebrew you can just run `brew install go`.

build:

```console
make build
cd ./bin/
```

run:

```console
./urlchecker --url extim.su
```

You can also check multiple urls in the same:

```console
./urlchecker --url extim.su,google.com:80,example.com:443
```

Yu can specify protocol (--protocol). It's can be tcp or udp.

```console
./urlchecker --url google.com:53 --protocol udp
```

Scanning list urls from file - url.txt and output as JSON format

```console
./urlchecker --file url.txt --json
```

### Docker

One url

```console
docker run --rm docker.io/extim/urlchecker --url extim.su
```

List of urls with custom ports

```console
docker run --rm docker.io/extim/urlchecker --url extim.su,google.com:80,example.com:443
```

List of urls with custom ports, and settings for default port

```console
docker run --rm docker.io/extim/urlchecker --url extim.su,google.com:80,example.com --port 443
```

Checking url with different protocol and JSON output

```console
docker run --rm docker.io/extim/urlchecker --url google.com:53 --protocol udp --json
```

Scanning list urls from file - url.txt

```console
docker run --rm -v ./urls.txt:/opt/urlchecker/bin/url.txt docker.io/extim/urlchecker --file url.txt
```
