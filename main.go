package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/itchyny/gojo"
	"github.com/urfave/cli/v3"
)

// version is overridden at build time via -ldflags "-X main.version=...".
var version = "dev"

var (
	baseURL  string
	username string
	password string
	insecure bool
)

func makeAction(method string) cli.ActionFunc {
	return func(ctx context.Context, cmd *cli.Command) error {
		out := cmd.Root().Writer
		if out == nil {
			out = os.Stdout
		}
		errOut := cmd.Root().ErrWriter
		if errOut == nil {
			errOut = os.Stderr
		}

		args := cmd.Args().Slice()
		if len(args) == 0 {
			return fmt.Errorf("path required")
		}
		path, kvArgs := args[0], args[1:]

		restURL, err := url.JoinPath(baseURL, "rest", path)
		if err != nil {
			return fmt.Errorf("error: %w", err)
		}

		var body io.Reader
		if method == http.MethodPut || method == http.MethodPatch || method == http.MethodPost {
			if len(kvArgs) > 0 {
				// gojo.Map converts ["k=v", ...] → {"k": "v", ...}
				m, err := gojo.Map(kvArgs)
				if err != nil {
					return fmt.Errorf("parse args: %w", err)
				}
				data, err := json.Marshal(m)
				if err != nil {
					return err
				}
				body = bytes.NewReader(data)
			} else {
				body = bytes.NewReader([]byte("{}"))
			}
		} else if method == http.MethodGet && len(kvArgs) > 0 {
			u, err := url.Parse(restURL)
			if err != nil {
				return err
			}
			q := url.Values{}
			for _, arg := range kvArgs {
				k, v, _ := strings.Cut(arg, "=")
				q.Set(k, v)
			}
			u.RawQuery = q.Encode()
			restURL = u.String()
		}

		req, err := http.NewRequestWithContext(ctx, method, restURL, body)
		if err != nil {
			return err
		}
		creds := base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
		req.Header.Set("Authorization", "Basic "+creds)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		client := &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: insecure}, //nolint:gosec
				Proxy:           http.ProxyFromEnvironment,
			},
		}

		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}

		if resp.StatusCode >= 400 {
			fmt.Fprintf(errOut, "error: %s\n%s\n", resp.Status, respBody)
			return fmt.Errorf("HTTP %d", resp.StatusCode)
		}

		if len(respBody) == 0 {
			return nil
		}

		var v any
		if err := json.Unmarshal(respBody, &v); err != nil {
			out.Write(respBody)
			return nil
		}
		pretty, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			return err
		}
		fmt.Fprintln(out, string(pretty))
		return nil
	}
}

func newCommand() *cli.Command {
	return &cli.Command{
		Name:    "ros",
		Usage:   "RouterOS REST API client",
		Version: version,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "url",
				Aliases:     []string{"H"},
				Usage:       "RouterOS base URL (e.g. https://192.168.88.1)",
				Sources:     cli.EnvVars("ROUTEROS_URL"),
				Required:    true,
				Destination: &baseURL,
			},
			&cli.StringFlag{
				Name:        "username",
				Aliases:     []string{"u"},
				Usage:       "HTTP Basic Auth username",
				Sources:     cli.EnvVars("ROUTEROS_USERNAME"),
				Required:    true,
				Destination: &username,
			},
			&cli.StringFlag{
				Name:        "password",
				Aliases:     []string{"p"},
				Usage:       "HTTP Basic Auth password",
				Sources:     cli.EnvVars("ROUTEROS_PASSWORD"),
				Required:    true,
				Destination: &password,
			},
			&cli.BoolFlag{
				Name:        "insecure",
				Aliases:     []string{"k"},
				Usage:       "skip TLS certificate verification",
				Sources:     cli.EnvVars("ROUTEROS_INSECURE"),
				Destination: &insecure,
			},
		},
		Commands: []*cli.Command{
			{Name: "get", Usage: "read records", Action: makeAction(http.MethodGet)},
			{Name: "put", Usage: "create a record", Action: makeAction(http.MethodPut)},
			{Name: "patch", Usage: "update a record", Action: makeAction(http.MethodPatch)},
			{Name: "delete", Usage: "delete a record", Action: makeAction(http.MethodDelete)},
			{Name: "post", Usage: "run any command", Action: makeAction(http.MethodPost)},
		},
	}
}

func main() {
	if err := newCommand().Run(context.Background(), os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
