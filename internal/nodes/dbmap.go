// SPDX-License-Identifier: AGPL-3.0-only

package nodes

import (
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
)

func mapDB(err error) *httpError {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return nil
	}
	switch pgErr.Code {
	case "23505":
		switch {
		case strings.Contains(pgErr.ConstraintName, "slug"):
			return conflict("kind slug already exists")
		case strings.Contains(pgErr.ConstraintName, "key"):
			return conflict("node key already exists")
		default:
			return conflict("conflict")
		}
	case "23503":
		if strings.Contains(pgErr.ConstraintName, "kind") {
			return badRequest("kind not found")
		}
		return badRequest("related row does not exist")
	case "23514":
		return badRequest("invalid value")
	case "22P02":
		return badRequest("bad request")
	case "P0001":
		switch pgErr.Message {
		case "node has live children":
			return conflict("node has live children")
		case "child kind is not allowed under parent kind":
			return conflict("child kind is not allowed under parent kind")
		case "node move would create a cycle":
			return conflict("node move would create a cycle")
		case "node cannot parent itself":
			return conflict("node cannot parent itself")
		case "parent node does not exist or is deleted":
			return badRequest("parent node does not exist or is deleted")
		case "cannot restore node under deleted parent":
			return conflict("cannot restore node under deleted parent")
		case "invalid tenant or key prefix":
			return badRequest("invalid key prefix")
		default:
			return nil
		}
	default:
		return nil
	}
}

func dbErr(op string, err error) error {
	if err == nil {
		return nil
	}
	if he := mapDB(err); he != nil {
		return he
	}
	return fmt.Errorf("%s: %w", op, err)
}
