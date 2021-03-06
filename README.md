# lokitee

`lokitee` streams lines of text from stdin, forwarding it to stdout and to
Grafana Loki.

Credentials to Loki can be set via flag or environment variable:

* `-addr` or `$LOKI_ADDR`: Base URL of Loki to connect to (i.e., `http://localhost:8080`)
* `-username` or `$LOKI_USERNAME`: Basic auth username to use for requests. Optional.
* `-password` or `$LOKI_PASSWORD`: Basic auth password to use for requests. Optional.

By default, sent logs are written with the label set `{job="lokitee"}`. This
can be changed with the `-labels` flag.

## Installing

Use Go to install:

```
go install github.com/rfratto/lokitee@main
```

## Examples

Write `Hello, world` to Loki:

```
echo "Hello, world!" | lokitee
```

## Gotchas

`lokitee` currently splits lines from stdin and sends each line to Loki,
preventing you from having one log entry that spans multiple lines.
