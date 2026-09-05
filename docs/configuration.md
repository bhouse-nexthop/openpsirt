# Configuration

Every setting comes from the environment, and every name starts with
`OPENPSIRT_`. Each one has a working default, so the process starts with
nothing set but a database — and a value that is set and cannot be read stops
the process with the variable named, rather than falling back to the default.
A switch spelled wrongly that silently reads as its opposite is worse than a
refusal to start.

A switch takes `true` or `false` (also `1`, `0`, `t`, `f`, in any case). A
duration is written as Go reads it: `30s`, `5m`, `12h`. A number is a positive
whole number; zero reads as unset everywhere, so it is refused rather than
taken.

## Serving

| Variable | Meaning | Default |
|---|---|---|
| `OPENPSIRT_ADDR` | The `host:port` the HTTP server listens on | `:8080` |
| `OPENPSIRT_BASE_URL` | The address people arrive on. Behind a proxy that is not what the process thinks it is called, and a sign-in provider compares the address it sends people back to against what it was registered with, so it is stated rather than guessed. Required once a provider is configured | unset |
| `OPENPSIRT_PLAIN_HTTP` | Serve without TLS, which is what running locally looks like. It only loosens cookies: the session cookie is sent over plain HTTP, which it otherwise is not | `false` |
| `OPENPSIRT_SHUTDOWN_GRACE` | How long requests in flight get to finish on a stop signal, and then how long background work gets to finish after that | `15s` |
| `OPENPSIRT_LOG_LEVEL` | `debug`, `info`, `warn` or `error` | `info` |
| `OPENPSIRT_LOG_FORMAT` | `text` or `json` | `text` |

## Database

| Variable | Meaning | Default |
|---|---|---|
| `OPENPSIRT_DATABASE_URL` | Which database and how to reach it: `postgres://user:password@host:5432/name`, `mysql://…`, `mariadb://…`, or `sqlite:///absolute/path.db`. SQLite is for development and a single-pod trial, never production. **Required** | unset |
| `OPENPSIRT_AUTO_MIGRATE` | Apply outstanding schema changes at startup, so deploying the binary is the whole upgrade. Turn it off to run `openpsirt migrate up` yourself, under different credentials, at a time you choose | `true` |
| `OPENPSIRT_DB_MAX_OPEN` | Most connections open at once | `25` |
| `OPENPSIRT_DB_MAX_IDLE` | Most connections kept open idle | `25` |
| `OPENPSIRT_DB_IDLE_TIMEOUT` | How long an idle connection is kept before it is closed. Shorter than anything between the process and the server would close it, so nothing closes one behind the process's back | `1m` |
| `OPENPSIRT_DB_CONN_LIFETIME` | How long a connection is used before it is replaced | `30m` |

## Scanning

| Variable | Meaning | Default |
|---|---|---|
| `OPENPSIRT_SCANNER_PATH` | Where the vulnerability scanner binary lives. Empty means whatever the environment resolves. The scanner is a requirement of a deployment rather than an option: the vulnerability data is produced here, not sent in | unset |

## Telling people

Mail is how anything leaves the application. A deployment that sets none of
this tells nobody anything outside it, which is an ordinary way to run: the
notification area in the interface still works.

`MAIL_FROM` and `MAIL_SERVER` are both needed for mail to happen at all — a
server with nobody to send as is half a configuration, and either alone sends
nothing rather than failing at startup. Credentials are optional, and are
refused over a connection the server would not secure with STARTTLS.

| Variable | Meaning | Default |
|---|---|---|
| `OPENPSIRT_MAIL_FROM` | The address messages are sent as. Set it and the server together, or neither | unset |
| `OPENPSIRT_MAIL_SERVER` | The SMTP server as `host:port`, e.g. `smtp.example.com:587` | unset |
| `OPENPSIRT_MAIL_USERNAME` | Username, where the server wants one. Sent only after STARTTLS | unset |
| `OPENPSIRT_MAIL_PASSWORD` | Password for that username. Sent only after STARTTLS | unset |

Who gets what is not configuration: each person chooses in their own settings,
and the daily digest is off until somebody asks for it. A message about a
finding nobody has announced carries no detail — only that there is something,
and a link.

On the Helm chart these are the `mail` values, and the password goes in a
Secret the chart makes or one you name.

## Publishing advisories

An advisory is a document about a flaw in your own product. It is generated
from what this deployment already holds and handed to you; nothing is sent
anywhere, and nothing records that you published it.

Both a name and a namespace are needed for either to do anything. A CSAF
document requires a publisher, so with one missing no advisory is generated and
the refusal says which — rather than handing you a document that fails
validation after you have sent it.

| Variable | Meaning | Default |
|---|---|---|
| `OPENPSIRT_PUBLISHER_NAME` | The organization advisories say issued them. Set it and the namespace together, or neither | unset |
| `OPENPSIRT_PUBLISHER_NAMESPACE` | A URL identifying that organization, which is what a reader of a CSAF document matches on | unset |
| `OPENPSIRT_PUBLISHER_CATEGORY` | What the standard calls the kind of publisher. A deployment publishing about its own product is a vendor | `vendor` |

On the Helm chart these go through `extraEnv`, since a deployment that does not
publish needs none of them.

## Who may sign in

The process refuses to start until somebody can administer it, and naming
somebody grants a role — it does not let anybody in without signing in.

Configure at least one of the sign-in methods below, or nobody can reach it.
**The Helm chart refuses to render an install with none; the binary does not
check**, because a deployment being brought up in pieces is an ordinary state
for a process and not for an install.

| Variable | Meaning | Default |
|---|---|---|
| `OPENPSIRT_BOOTSTRAP_ADMINS` | Identities granted administration at every startup, comma-separated. Applied every time rather than only the first, so it is the way back in for an operator who has locked themselves out: add yourself, restart | unset |
| `OPENPSIRT_SESSION_LIFETIME` | How long a sign-in lasts. Unset takes the built-in default, which an administrator may change in the settings; a value set here has to be a positive duration | 12 hours |

### An OpenID Connect provider

| Variable | Meaning | Default |
|---|---|---|
| `OPENPSIRT_OIDC_ISSUER` | The provider's issuer address. Empty means no provider | unset |
| `OPENPSIRT_OIDC_NAME` | What the sign-in button calls it | `oidc` |
| `OPENPSIRT_OIDC_CLIENT_ID` | The client registered with the provider | unset |
| `OPENPSIRT_OIDC_CLIENT_SECRET` | Its secret | unset |
| `OPENPSIRT_OIDC_USERNAME_CLAIM` | The claim to take a person's identity from, when it is not the subject | unset |
| `OPENPSIRT_OIDC_GROUPS_CLAIM` | The claim carrying group membership, if the provider asserts it | unset |

### GitHub

| Variable | Meaning | Default |
|---|---|---|
| `OPENPSIRT_GITHUB_CLIENT_ID` | The OAuth application's client id. Empty means GitHub sign-in is off | unset |
| `OPENPSIRT_GITHUB_CLIENT_SECRET` | Its secret | unset |
| `OPENPSIRT_GITHUB_ORG` | Restrict sign-in to members of one organization, and read its teams as groups. Empty means anybody with a GitHub account, which is rarely what you want | unset |

### A proxy that says who somebody is

Both the header and the sources it is believed from are required together: a
header named with nothing to trust it from is either a mistake or the first
half of one, and the process stops rather than accept a header anybody can set.

| Variable | Meaning | Default |
|---|---|---|
| `OPENPSIRT_TRUSTED_HEADER` | The header an identity-aware proxy sets to say who somebody is | unset |
| `OPENPSIRT_TRUSTED_SOURCES` | Addresses or CIDR ranges the header is believed from, comma-separated. Anything else presenting it is ignored | unset |
| `OPENPSIRT_TRUSTED_GROUPS_HEADER` | Where that proxy reports group membership, if it does | unset |
| `OPENPSIRT_TRUSTED_GROUPS_DELIMITER` | What separates the names in it. Neither the header nor the separator is standardized, so both are named rather than guessed | `,` |
