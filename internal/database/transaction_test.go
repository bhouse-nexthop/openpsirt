package database_test

import (
	"errors"
	"testing"

	"github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/bhouse-nexthop/openpsirt/internal/database"
)

func TestLosingARaceIsWorthRetryingAndAMistakeIsNot(t *testing.T) {
	// The distinction the whole retry rests on. A conflict wrongly called
	// permanent surfaces as an error somebody reads; a mistake wrongly
	// retried surfaces as a deployment hammering its database and hanging.
	//
	// The clustered case is the one that motivated this: a cluster certifies a
	// write at COMMIT, and reports the failure as a deadlock — on a
	// transaction whose every statement had already succeeded.
	worth := []error{
		&mysql.MySQLError{Number: 1213, Message: "Deadlock found when trying to get lock"},
		&mysql.MySQLError{Number: 1205, Message: "Lock wait timeout exceeded"},
		&mysql.MySQLError{Number: 1180, Message: "Got error during COMMIT"},
		&pgconn.PgError{Code: "40001", Message: "could not serialize access"},
		&pgconn.PgError{Code: "40P01", Message: "deadlock detected"},
		errors.New("database is locked (5) (SQLITE_BUSY)"),
	}
	for _, err := range worth {
		if !database.WorthRetrying(err) {
			t.Errorf("not retried: %v", err)
		}
	}

	// Everything else is somebody's mistake, and going again will not fix it.
	notWorth := []error{
		nil,
		errors.New("some other trouble"),
		&mysql.MySQLError{Number: 1062, Message: "Duplicate entry"},
		&mysql.MySQLError{Number: 1452, Message: "Cannot add or update a child row"},
		&pgconn.PgError{Code: "23505", Message: "duplicate key value violates unique constraint"},
		&pgconn.PgError{Code: "23503", Message: "violates foreign key constraint"},
		&pgconn.PgError{Code: "42P01", Message: "relation does not exist"},
	}
	for _, err := range notWorth {
		if database.WorthRetrying(err) {
			t.Errorf("retried something that will never come out differently: %v", err)
		}
	}
}

func TestAWrappedFailureIsStillRecognized(t *testing.T) {
	// Every store wraps what the driver returned, so a check that only matched
	// a bare error would recognize none of them.
	wrapped := errors.Join(
		errors.New("record this sign-in"),
		&mysql.MySQLError{Number: 1213, Message: "Deadlock found"},
	)
	if !database.WorthRetrying(wrapped) {
		t.Error("a wrapped deadlock was not recognized")
	}
}
