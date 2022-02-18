# lokitee

`lokitee` lets you tee arguments or the contents of stdin to Loki.

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
lokitee Hello, world
```

Pipe the output of `cat main.go` to Loki:

```
cat main.go | lokitee
```

## Gotcha

`lokitee` has no retry logic if a request to Loki fails.
