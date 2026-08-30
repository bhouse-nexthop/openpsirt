// Package database opens and describes the supported databases.
//
// Four engines are supported and all four are tested. Queries elsewhere in the
// application are written to run unchanged on every one of them; the
// engine-specific parts are confined here, to schema migration, and to the job
// queue's locking.
package database

import (
	"fmt"
	"net/url"
	"strings"
)

// Engine is a supported database.
type Engine string

const (
	// Postgres is PostgreSQL.
	Postgres Engine = "postgres"
	// MySQL is Oracle MySQL.
	MySQL Engine = "mysql"
	// MariaDB is MariaDB. It shares a wire protocol and driver with MySQL but
	// has diverged enough — in JSON handling, sequences and partitioning — to
	// be treated and tested as its own target.
	MariaDB Engine = "mariadb"
	// SQLite is for development and testing only. It is never a production
	// target, so anything that only works there is a bug.
	SQLite Engine = "sqlite"
)

// String returns the engine's name.
func (e Engine) String() string { return string(e) }

// IsProduction reports whether the engine is supported for production use.
func (e Engine) IsProduction() bool { return e != SQLite && e != "" }

// engineFromScheme maps the scheme of a database URL onto an engine.
//
// MySQL and MariaDB share a scheme because they share a driver; which one is
// actually answering is settled after connecting, by asking the server.
var engineFromScheme = map[string]Engine{
	"postgres":   Postgres,
	"postgresql": Postgres,
	"mysql":      MySQL,
	"mariadb":    MariaDB,
	"sqlite":     SQLite,
	"sqlite3":    SQLite,
	"file":       SQLite,
}

// Target is a parsed database URL: which engine, and how to reach it.
type Target struct {
	// Engine is the engine named by the URL. For MySQL and MariaDB this is
	// provisional until the server has been asked — see Detect.
	Engine Engine
	// DSN is the connection string in the form the driver expects, which is
	// not always the URL that was supplied.
	DSN string
	// Redacted is the URL with any password removed, safe to log.
	Redacted string
}

// ParseURL turns a database URL into something a driver can open.
//
// Supported forms:
//
//	postgres://user:password@host:5432/database
//	mysql://user:password@host:3306/database
//	mariadb://user:password@host:3306/database
//	sqlite:///absolute/path.db
//	sqlite://:memory:
func ParseURL(raw string) (Target, error) {
	if strings.TrimSpace(raw) == "" {
		return Target{}, fmt.Errorf("database URL is empty")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return Target{}, fmt.Errorf("database URL is not a URL: %w", err)
	}
	engine, ok := engineFromScheme[strings.ToLower(u.Scheme)]
	if !ok {
		return Target{}, fmt.Errorf("unsupported database %q: want one of postgres, mysql, mariadb, sqlite", u.Scheme)
	}
	// Normalize the scheme into the URL the driver receives. Accepting
	// "POSTGRES://" and passing it through unchanged had pgx reject it and
	// silently fall back to environment defaults, producing an error that
	// named the supplied URL while describing a connection somewhere else.
	u.Scheme = strings.ToLower(u.Scheme)
	raw = u.String()

	dsn, err := driverDSN(engine, u, raw)
	if err != nil {
		return Target{}, err
	}
	return Target{Engine: engine, DSN: dsn, Redacted: redact(u)}, nil
}

func driverDSN(engine Engine, u *url.URL, raw string) (string, error) {
	switch engine {
	case Postgres:
		// pgx accepts the URL as given.
		return raw, nil

	case MySQL, MariaDB:
		// The MySQL driver wants user:pass@tcp(host:port)/db, not a URL.
		database := strings.TrimPrefix(u.Path, "/")
		if database == "" {
			return "", fmt.Errorf("database URL names no database")
		}
		host := u.Host
		if host == "" {
			host = "127.0.0.1:3306"
		} else if !strings.Contains(host, ":") {
			host += ":3306"
		}
		var credentials string
		if u.User != nil {
			if password, set := u.User.Password(); set {
				credentials = u.User.Username() + ":" + password + "@"
			} else {
				credentials = u.User.Username() + "@"
			}
		}
		query := u.RawQuery
		// Times must come back as time.Time rather than []byte, and in UTC,
		// or every timestamp comparison depends on the server's timezone.
		//
		// ANSI_QUOTES is the other half, and it is about identifiers. These
		// two engines quote with backticks by default, where the other two use
		// the standard double quote — so without this, writing portable data
		// definition means either quoting nothing or writing it twice.
		//
		// Quoting nothing is what a reserved word catches: a column named for
		// something that later becomes a function is refused outright, and the
		// engines do not agree on which words those are. Turning this on lets
		// every identifier be quoted the same way everywhere, which makes the
		// question stop arising.
		//
		// It is additive. Backticks keep working, so anything generating them
		// is unaffected, and string literals are untouched — this changes what
		// a double quote means, not what a quote means.
		settings := "parseTime=true&loc=UTC&sql_mode=%27ANSI_QUOTES%27"
		if query != "" {
			query += "&" + settings
		} else {
			query = settings
		}
		return fmt.Sprintf("%stcp(%s)/%s?%s", credentials, host, database, query), nil

	case SQLite:
		// sqlite://:memory: and sqlite:///path both have to work. url.Parse
		// puts ":memory:" in Opaque and a path in Path.
		var path string
		switch {
		case u.Opaque != "":
			path = strings.TrimPrefix(u.Opaque, "//")
		case u.Host == ":memory:":
			path = ":memory:"
		case u.Host != "":
			// "sqlite://data/file.db" parses with "data" as the host, and
			// silently dropping it wrote to the filesystem root instead. A
			// relative path needs "sqlite:data/file.db"; an absolute one needs
			// three slashes. Say so rather than writing somewhere unexpected.
			return "", fmt.Errorf(
				"ambiguous sqlite URL %q: use sqlite:%s%s for a relative path, or sqlite:///%s for an absolute one",
				raw, u.Host, u.Path, strings.TrimPrefix(u.Path, "/"))
		case u.Path != "":
			path = u.Path
		default:
			return "", fmt.Errorf("database URL names no file")
		}
		// Wait for a held lock rather than failing at once. Without this,
		// anything concurrent gets an immediate "database is locked" instead
		// of queueing, which looks like a bug and is really impatience.
		return path + "?_pragma=busy_timeout(10000)&_pragma=foreign_keys(1)", nil
	}
	return "", fmt.Errorf("unsupported database %q", engine)
}

// secretParams are query parameters that carry a credential. Drivers accept
// passwords this way as well as in the userinfo, and a redaction that only
// handles userinfo puts the password in the first log line of every start.
var secretParams = []string{"password", "sslpassword", "sslkey"}

func redact(u *url.URL) string {
	clone := *u
	if u.User != nil {
		if _, set := u.User.Password(); set {
			clone.User = url.UserPassword(u.User.Username(), "xxxxx")
		}
	}
	if q := clone.Query(); len(q) > 0 {
		changed := false
		for _, name := range secretParams {
			if q.Has(name) {
				q.Set(name, "xxxxx")
				changed = true
			}
		}
		if changed {
			clone.RawQuery = q.Encode()
		}
	}
	return clone.String()
}
