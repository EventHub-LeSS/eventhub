package repository

import (
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
)

// ErrUnknownReference means a write referenced a row that does not exist, e.g. an event whose
// category or location is unknown. It is a client error, not a database failure.
var ErrUnknownReference = errors.New("unknown reference")

// sqlStateForeignKeyViolation is the Postgres error code for foreign_key_violation.
const sqlStateForeignKeyViolation = "23503"

// eventReferenceFields names the request field behind each foreign key a client sets on events.
// The organizer is not listed: the API derives it from the token, so a violation there is a server error.
var eventReferenceFields = map[string]string{
	"events_category_id_fkey": "categoryId",
	"events_location_id_fkey": "locationId",
}

func translateEventWriteError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == sqlStateForeignKeyViolation {
		if field, ok := eventReferenceFields[pgErr.ConstraintName]; ok {
			return fmt.Errorf("%w: %s does not exist", ErrUnknownReference, field)
		}
	}
	return err
}
