package database

import "testing"

func TestParseURLRecognizesEachEngine(t *testing.T) {
	for _, tc := range []struct {
		url    string
		engine Engine
	}{
		{"postgres://u:p@host:5432/db", Postgres},
		{"postgresql://u:p@host:5432/db", Postgres},
		{"mysql://u:p@host:3306/db", MySQL},
		{"mariadb://u:p@host:3306/db", MariaDB},
		{"sqlite:///var/lib/openpsirt.db", SQLite},
		{"sqlite://:memory:", SQLite},
	} {
		t.Run(tc.url, func(t *testing.T) {
			got, err := ParseURL(tc.url)
			if err != nil {
				t.Fatalf("ParseURL: %v", err)
			}
			if got.Engine != tc.engine {
				t.Errorf("engine = %q, want %q", got.Engine, tc.engine)
			}
			if got.DSN == "" {
				t.Error("DSN is empty")
			}
		})
	}
}

func TestParseURLRejectsWhatWeDoNotSupport(t *testing.T) {
	for _, raw := range []string{
		"",
		"  ",
		"oracle://u:p@host/db",
		"mongodb://host/db",
		"mysql://u:p@host:3306/", // names no database
		"sqlite://",              // names no file
	} {
		if _, err := ParseURL(raw); err == nil {
			t.Errorf("ParseURL(%q) was accepted", raw)
		}
	}
}

func TestMySQLDSNIsRewrittenForItsDriver(t *testing.T) {
	// The driver wants user:pass@tcp(host:port)/db, not a URL. Handing it the
	// URL unchanged fails at connect time with an unhelpful message.
	got, err := ParseURL("mysql://user:secret@db.example:3306/openpsirt")
	if err != nil {
		t.Fatalf("ParseURL: %v", err)
	}
	want := "user:secret@tcp(db.example:3306)/openpsirt?parseTime=true&loc=UTC&sql_mode=%27ANSI_QUOTES%27"
	if got.DSN != want {
		t.Errorf("DSN\n got %q\nwant %q", got.DSN, want)
	}
}

func TestMySQLDSNAlwaysAsksForUTCTimesAndStandardQuoting(t *testing.T) {
	// Without the first two the driver returns timestamps as bytes in the
	// server's timezone, so every time comparison depends on how the server is
	// set up.
	//
	// The third is about identifiers. These engines quote with backticks by
	// default where the other two use the standard double quote, so without it
	// portable data definition means quoting nothing — and quoting nothing is
	// what a reserved word catches, on whichever engine happens to reserve it.
	got, err := ParseURL("mysql://u@h/db?charset=utf8mb4")
	if err != nil {
		t.Fatalf("ParseURL: %v", err)
	}
	for _, want := range []string{"charset=utf8mb4", "parseTime=true", "loc=UTC", "ANSI_QUOTES"} {
		if !contains(got.DSN, want) {
			t.Errorf("DSN %q is missing %q", got.DSN, want)
		}
	}
}

func TestPasswordsAreNotLogged(t *testing.T) {
	got, err := ParseURL("postgres://user:hunter2@host:5432/db")
	if err != nil {
		t.Fatalf("ParseURL: %v", err)
	}
	if contains(got.Redacted, "hunter2") {
		t.Errorf("redacted form still contains the password: %q", got.Redacted)
	}
	if !contains(got.DSN, "hunter2") {
		t.Error("the DSN lost the password; it is what actually connects")
	}
}

func TestOnlySQLiteIsNonProduction(t *testing.T) {
	for _, e := range []Engine{Postgres, MySQL, MariaDB} {
		if !e.IsProduction() {
			t.Errorf("%s should be a production engine", e)
		}
	}
	if SQLite.IsProduction() {
		t.Error("SQLite must never count as a production engine")
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (func() bool {
		for i := 0; i+len(needle) <= len(haystack); i++ {
			if haystack[i:i+len(needle)] == needle {
				return true
			}
		}
		return false
	})()
}
