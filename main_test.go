package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"
)

// capturedRequest records what the CLI sent to the server.
type capturedRequest struct {
	method      string
	path        string
	rawQuery    string
	body        []byte
	authHeader  string
	contentType string
}

// newServer returns an httptest server that records the incoming request into
// got and replies with the given status and body.
func newServer(t *testing.T, status int, respBody string, got *capturedRequest) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		*got = capturedRequest{
			method:      r.Method,
			path:        r.URL.Path,
			rawQuery:    r.URL.RawQuery,
			body:        body,
			authHeader:  r.Header.Get("Authorization"),
			contentType: r.Header.Get("Content-Type"),
		}
		w.WriteHeader(status)
		io.WriteString(w, respBody)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// run invokes the CLI against baseURL with the given arguments, capturing
// stdout and stderr. Global auth flags are always supplied.
func run(t *testing.T, baseURL string, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	cmd := newCommand()
	var out, errOut bytes.Buffer
	cmd.Writer = &out
	cmd.ErrWriter = &errOut
	full := append([]string{"ros", "--url", baseURL, "-u", "user", "-p", "pass"}, args...)
	err = cmd.Run(context.Background(), full)
	return out.String(), errOut.String(), err
}

func wantBasicAuth(t *testing.T, got string) {
	t.Helper()
	want := "Basic " + base64.StdEncoding.EncodeToString([]byte("user:pass"))
	if got != want {
		t.Errorf("Authorization header = %q, want %q", got, want)
	}
}

// assertJSONBody compares the captured request body against want, ignoring key
// order.
func assertJSONBody(t *testing.T, got []byte, want string) {
	t.Helper()
	var gotVal, wantVal any
	if err := json.Unmarshal(got, &gotVal); err != nil {
		t.Fatalf("response body is not valid JSON: %v (%q)", err, got)
	}
	if err := json.Unmarshal([]byte(want), &wantVal); err != nil {
		t.Fatalf("bad want JSON in test: %v", err)
	}
	if !reflect.DeepEqual(gotVal, wantVal) {
		t.Errorf("body = %s, want %s", got, want)
	}
}

func TestGetJoinsPathAndSendsAuth(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `[{"id":"*1"}]`, &got)

	stdout, _, err := run(t, srv.URL, "get", "/ip/address")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.method != http.MethodGet {
		t.Errorf("method = %q, want GET", got.method)
	}
	if got.path != "/rest/ip/address" {
		t.Errorf("path = %q, want /rest/ip/address", got.path)
	}
	wantBasicAuth(t, got.authHeader)

	// Output is pretty-printed JSON.
	want := "[\n  {\n    \"id\": \"*1\"\n  }\n]\n"
	if stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
}

func TestGetExtraArgsBecomeQueryParams(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `[]`, &got)

	if _, _, err := run(t, srv.URL, "get", "/ip/address", "interface=ether1", "dynamic=false"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	q, err := url.ParseQuery(got.rawQuery)
	if err != nil {
		t.Fatal(err)
	}
	if q.Get("interface") != "ether1" {
		t.Errorf("interface = %q, want ether1", q.Get("interface"))
	}
	if q.Get("dynamic") != "false" {
		t.Errorf("dynamic = %q, want false", q.Get("dynamic"))
	}
	if len(got.body) != 0 {
		t.Errorf("GET should not send a body, got %q", got.body)
	}
}

func TestPutSendsJSONBody(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 201, `{}`, &got)

	if _, _, err := run(t, srv.URL, "put", "/ip/address", "address=1.2.3.4/24", "interface=bridge"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.method != http.MethodPut {
		t.Errorf("method = %q, want PUT", got.method)
	}
	if got.contentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got.contentType)
	}
	// Slash-bearing values stay strings (not coerced to numbers).
	assertJSONBody(t, got.body, `{"address":"1.2.3.4/24","interface":"bridge"}`)
}

func TestWriteWithoutArgsSendsEmptyObject(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, ``, &got)

	if _, _, err := run(t, srv.URL, "post", "/system/reboot"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertJSONBody(t, got.body, `{}`)
}

// TestBodyTypeInference pins gojo's behavior: bare numbers/booleans become JSON
// numbers/booleans, everything else stays a string.
func TestBodyTypeInference(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, ``, &got)

	if _, _, err := run(t, srv.URL, "post", "/ping", "address=1.1.1.1", "count=4", "verbose=true"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertJSONBody(t, got.body, `{"address":"1.1.1.1","count":4,"verbose":true}`)
}

// TestBodyNestingAndArrays pins gojo's bracket syntax for nested objects/arrays.
func TestBodyNestingAndArrays(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, ``, &got)

	if _, _, err := run(t, srv.URL, "put", "/x", "tags[]=red", "tags[]=blue", "meta[owner]=net"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertJSONBody(t, got.body, `{"tags":["red","blue"],"meta":{"owner":"net"}}`)
}

func TestErrorStatusReturnsError(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusBadRequest, `{"error":400,"message":"bad"}`, &got)

	stdout, stderr, err := run(t, srv.URL, "get", "/bogus")
	if err == nil {
		t.Fatal("expected an error for HTTP 400, got nil")
	}
	if stdout != "" {
		t.Errorf("stdout should be empty on error, got %q", stdout)
	}
	if !bytes.Contains([]byte(stderr), []byte("400")) {
		t.Errorf("stderr should mention the status, got %q", stderr)
	}
}

func TestNonJSONResponseIsPassedThrough(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, "plain text reply", &got)

	stdout, _, err := run(t, srv.URL, "get", "/x")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stdout != "plain text reply" {
		t.Errorf("stdout = %q, want raw passthrough", stdout)
	}
}

func TestEmptyResponseProducesNoOutput(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusNoContent, "", &got)

	stdout, _, err := run(t, srv.URL, "delete", "/ip/address/*9")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
	if got.method != http.MethodDelete {
		t.Errorf("method = %q, want DELETE", got.method)
	}
}

func TestPathRequired(t *testing.T) {
	// No server contacted: the action rejects a missing path first.
	_, _, err := run(t, "http://127.0.0.1:1", "get")
	if err == nil {
		t.Fatal("expected an error when no path is given")
	}
	if err.Error() != "path required" {
		t.Errorf("error = %q, want \"path required\"", err.Error())
	}
}
