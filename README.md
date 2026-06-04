# ros — RouterOS REST API client

[![CI](https://github.com/andreabedini/ros/actions/workflows/ci.yml/badge.svg)](https://github.com/andreabedini/ros/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/andreabedini/ros)](https://github.com/andreabedini/ros/releases)
[![Go Report Card](https://goreportcard.com/badge/github.com/andreabedini/ros)](https://goreportcard.com/report/github.com/andreabedini/ros)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

A minimal CLI for the [RouterOS REST API](https://help.mikrotik.com/docs/display/ROS/REST+API) (RouterOS v7.1+).

## Install

Download a prebuilt binary for your platform from the
[releases page](https://github.com/andreabedini/ros/releases), or install with Go:

```bash
go install github.com/andreabedini/ros@latest
```

To build from a checkout:

```bash
go build
```

### Verifying downloads

Release archives ship with [build provenance attestations](https://docs.github.com/en/actions/security-guides/using-artifact-attestations).
Verify a downloaded artifact against this repository with the GitHub CLI:

```bash
gh attestation verify ros_0.1.0_linux_amd64.tar.gz --repo andreabedini/ros
```

## Configuration

Credentials can be supplied via environment variables or flags (flags take precedence):

| Env var             | Flag              | Description                       |
|---------------------|-------------------|-----------------------------------|
| `ROUTEROS_URL`      | `-H`, `--url`     | Base URL, e.g. `https://router`   |
| `ROUTEROS_USERNAME` | `-u`, `--username`| HTTP Basic Auth username          |
| `ROUTEROS_PASSWORD` | `-p`, `--password`| HTTP Basic Auth password          |
| `ROUTEROS_INSECURE` | `-k`, `--insecure`| Skip TLS certificate verification |

If you front the router with a reverse proxy that injects the `Authorization`
header, point `ros` at the proxy and you can leave the credential flags unset.

## Commands

| Subcommand | HTTP method | RouterOS equivalent |
|------------|-------------|---------------------|
| `get`      | GET         | print               |
| `put`      | PUT         | add                 |
| `patch`    | PATCH       | set                 |
| `delete`   | DELETE      | remove              |
| `post`     | POST        | (any command)       |

### Path

The first positional argument is the RouterOS menu path, slash-separated (e.g. `/ip/address`). It maps directly to `<base-url>/rest<path>`.

### Arguments

**`get`** — extra `key=value` pairs become query string parameters (filter by property value):

```bash
ros get /ip/address interface=ether1
ros get /ip/address dynamic=false
```

**`put`, `patch`, `post`** — extra `key=value` pairs are serialised as a JSON body:

```bash
ros put /ip/address address=192.168.99.1/24 interface=dummy
ros patch /ip/address/.id=*3 comment=uplink
ros post /ping address=1.1.1.1 count=4
```

With no extra arguments, write commands send `{}` (useful for parameterless commands).

#### JSON body syntax

The body is built with [gojo](https://github.com/itchyny/gojo) (a Go port of
[`jo`](https://github.com/jpmens/jo)), so values are more than plain strings:

| Form                              | Becomes                                |
|-----------------------------------|----------------------------------------|
| `key=value`                       | `"value"` (string)                     |
| `count=4`, `ratio=1.5`            | `4`, `1.5` (number)                    |
| `enabled=true`, `x=null`          | `true`, `null` (boolean / null)        |
| `key="01"`                        | `"01"` (quotes force a string)         |
| `obj[k]=v`, `a[b][c]=1`           | `{"obj":{"k":"v"}}` (nested object)    |
| `tags[]=red tags[]=blue`          | `{"tags":["red","blue"]}` (array)      |

```bash
# Numbers and booleans are inferred from the value
ros post /ping address=1.1.1.1 count=4

# Force a string for numeric-looking values (note the shell quoting)
ros put /ip/pool ranges='"0102"'

# Nested objects and arrays via brackets
ros put /some/path 'meta[owner]=net' 'tags[]=lan' 'tags[]=trusted'
```

> **Heads-up:** bare values that look numeric are coerced — `001` becomes the
> number `1`. Wrap them in quotes (`key='"001"'`) to keep them as strings.
> Bracket characters (`[` `]`) are also shell metacharacters, so single-quote
> those arguments.

### Output

Responses are pretty-printed JSON. Errors print to stderr with the HTTP status; the process exits non-zero.

## Examples

```bash
# List all IP addresses
ros get /ip/address

# Read a single record by internal ID
ros get /ip/address/*3

# Add an IP address
ros put /ip/address address=10.0.0.1/24 interface=bridge

# Update a record
ros patch /ip/address/.id=*3 comment=uplink

# Delete a record
ros delete /ip/address/*9

# Run an arbitrary command (bounded to avoid the 60 s REST timeout)
ros post /ping address=1.1.1.1 count=4

# Change admin password
ros post /password old-password=old new-password=new confirm-new-password=new

# Monitor an LTE interface (once, to avoid streaming timeout)
ros post /interface/lte/monitor numbers=0 once=
```

## Releasing

Releases are automated with [release-please](https://github.com/googleapis/release-please)
and [GoReleaser](https://goreleaser.com). Use
[Conventional Commits](https://www.conventionalcommits.org) (`feat:`, `fix:`, …) on
`main`; release-please opens a release PR that bumps the version and updates the
changelog, and merging it tags the release and publishes binaries.

## License

[MIT](LICENSE)

