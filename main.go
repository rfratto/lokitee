package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"github.com/prometheus/prometheus/promql/parser"
)

func main() {
	var (
		lokiUrl   string
		username  string
		password  string
		rawLabels string
	)

	fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	fs.StringVar(&lokiUrl, "addr", "", "Server address. Defaults to $LOKI_ADDR if not set.")
	fs.StringVar(&username, "username", "", "Username for basic auth. Defaults to $LOKI_USERNAME if not set.")
	fs.StringVar(&password, "password", "", "Password for basic auth. Defaults to $LOKI_PASSWORD if not set.")
	fs.StringVar(&rawLabels, "labels", `{job="lokiecho"}`, `Labels to inject for logs. i.e., {app="shell"}`)

	if err := fs.Parse(os.Args[1:]); err != nil {
		abort("error: could not parse flags: %s", err)
	}

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

	input, err := argsOrStdin(fs)
	if err != nil {
		abort("error: could not get input: %s", err)
	}

	pushReq := pushRequest{
		Streams: []pushStream{{
			Stream: labels.Map(),
			Values: [][]string{{
				fmt.Sprintf("%d", time.Now().UnixNano()),
				input,
			}},
		}},
	}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	if err := enc.Encode(&pushReq); err != nil {
		abort("error: could not build request: %s", err)
	}

	parsedUrl, err := url.Parse(lokiUrl)
	if err != nil {
		abort("error: invalid url %s: %s", lokiUrl, err)
	}
	parsedUrl.Path = path.Join(parsedUrl.Path, "/loki/api/v1/push")

	req, err := http.NewRequest(http.MethodPost, parsedUrl.String(), bytes.NewReader(buf.Bytes()))
	if err != nil {
		abort("error: invalid http request: %s", err)
	}
	if username != "" || password != "" {
		req.SetBasicAuth(username, password)
	}
	req.Header.Add("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		abort("error: failed to perform http request: %s", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode/100 != 2 {
		bb, _ := io.ReadAll(resp.Body)
		abort("error: response %s from loki: %s", resp.Status, string(bb))
	}
}

func argsOrStdin(fs *flag.FlagSet) (string, error) {
	if fi, err := os.Stdin.Stat(); err == nil && fi.Mode()&os.ModeCharDevice == 0 {
		bb, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", err
		}
		return string(bb), nil
	}

	return strings.Join(fs.Args(), ` `), nil
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

type pushRequest struct {
	Streams []pushStream `json:"streams"`
}

type pushStream struct {
	Stream map[string]string `json:"stream"`
	Values [][]string        `json:"values"`
}
