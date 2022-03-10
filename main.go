package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/signal"
	"path"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/backoff"
	"github.com/grafana/dskit/flagext"
	"github.com/grafana/loki/clients/pkg/promtail/api"
	"github.com/grafana/loki/clients/pkg/promtail/client"
	"github.com/grafana/loki/pkg/logproto"
	"github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/promql/parser"
)

func main() {
	var (
		lokiUrl              string
		username             string
		password             string
		rawLabels            string
		interruptWaitSeconds int64
	)

	fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	fs.StringVar(&lokiUrl, "addr", "", "Server address. Defaults to $LOKI_ADDR if not set.")
	fs.StringVar(&username, "username", "", "Username for basic auth. Defaults to $LOKI_USERNAME if not set.")
	fs.StringVar(&password, "password", "", "Password for basic auth. Defaults to $LOKI_PASSWORD if not set.")
	fs.StringVar(&rawLabels, "labels", `{job="lokitee"}`, `Labels to inject for logs. i.e., {app="shell"}`)
	fs.Int64Var(&interruptWaitSeconds, "interrupt-wait", 0, "Number of seconds to delay exiting if sending an interrupt signal like SIGTERM is sent")

	if err := fs.Parse(os.Args[1:]); err != nil {
		abort("error: could not parse flags: %s", err)
	}

	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt)
		<-c
		time.Sleep(time.Duration(interruptWaitSeconds) * time.Second)
	}()

	lokiUrl = stringOrDefault(lokiUrl, os.Getenv("LOKI_ADDR"))
	username = stringOrDefault(username, os.Getenv("LOKI_USERNAME"))
	password = stringOrDefault(password, os.Getenv("LOKI_PASSWORD"))

	if lokiUrl == "" {
		abort("error: --addr must be provided or $LOKI_ADDR must be set")
	}
	if (username != "" || password != "") && (username == "" || password == "") {
		abort("error: username and password must be set together, but one is unset")
	}

	labels, err := parser.ParseMetric(rawLabels)
	if err != nil {
		abort("error: could not parse -labels: %s", err)
	}

	parsedUrl, err := url.Parse(lokiUrl)
	if err != nil {
		abort("error: invalid url %s: %s", lokiUrl, err)
	}
	parsedUrl.Path = path.Join(parsedUrl.Path, "/loki/api/v1/push")

	// We want to stream logs from stdin to both Loki and stdout. To write to
	// Loki as efficiently as possible, we create a Promtail client and rely on
	// its native support for batching.

	// Data sent to the Promtail client is handled in the background, and errors
	// are only exposed via logger; create a logger for Promtail to report errors
	// to.
	logger := log.NewLogfmtLogger(log.NewSyncWriter(os.Stderr))
	logger = log.With(logger, "program", "lokitee")
	logger = level.NewFilter(logger, level.AllowError())

	cliConfig := client.Config{
		URL:       flagext.URLValue{URL: parsedUrl},
		BatchWait: client.BatchWait,
		BatchSize: client.BatchSize,

		BackoffConfig: backoff.Config{
			MinBackoff: client.MinBackoff,
			MaxBackoff: client.MaxBackoff,
			MaxRetries: client.MaxRetries,
		},

		Timeout:  client.Timeout,
		TenantID: "", // TODO(rfratto): make configurable
	}
	if username != "" || password != "" {
		// TODO(rfratto): configure other types of credentials (eg Bearer auth)?
		cliConfig.Client = config.HTTPClientConfig{
			BasicAuth: &config.BasicAuth{
				Username: username,
				Password: config.Secret(password),
			},
		}
	}
	cli, err := client.New(nil, cliConfig, logger)
	if err != nil {
		abort("error: creating promtail client: %s", err)
	}
	defer cli.Stop()

	lw := promtailWriter{
		labels: toLabelSet(labels),
		write:  cli.Chan(),
	}

	// Tee to stdout and our promtail client. We wrap stdout in a lineWriter so
	// newlines are re-injected just for terminal output.
	mw := io.MultiWriter(&lineWriter{next: os.Stdout}, &lw)

	// Scan over stdin and send every line to Loki.
	//
	// TODO(rfratto): should teeing work another way? What if the user doesn't
	// want to send each read line as an individual log?
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		_, err := fmt.Fprint(mw, scanner.Text())
		if err != nil {
			fmt.Fprintln(os.Stderr, "failed writing: %w", err)
		}
	}
	if err := scanner.Err(); err != nil {
		abort(err.Error())
	}
}

func toLabelSet(in labels.Labels) model.LabelSet {
	res := make(model.LabelSet, len(in))
	for _, pair := range in {
		res[model.LabelName(pair.Name)] = model.LabelValue(pair.Value)
	}
	return res
}

type promtailWriter struct {
	labels model.LabelSet
	write  chan<- api.Entry
}

func (lw *promtailWriter) Write(bb []byte) (int, error) {
	lw.write <- api.Entry{
		Labels: lw.labels,
		Entry: logproto.Entry{
			Timestamp: time.Now().UTC(),
			Line:      string(bb),
		},
	}
	return len(bb), nil
}

type lineWriter struct {
	next io.Writer
}

func (lw lineWriter) Write(bb []byte) (int, error) {
	n, err := lw.next.Write(bb)
	_, _ = lw.next.Write([]byte{'\n'})
	return n, err
}

func abort(msg string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, msg, args...)
	os.Exit(1)
}

func stringOrDefault(value string, def string) string {
	if value != "" {
		return value
	}
	return def
}
