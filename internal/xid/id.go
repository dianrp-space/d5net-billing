package xid

import (
	"github.com/google/uuid"
)

// ID is a random UUID primary/foreign key.
type ID = uuid.UUID

func New() ID {
	return uuid.New()
}

func Nil() ID {
	return uuid.Nil
}

func Parse(s string) (ID, error) {
	return uuid.Parse(s)
}

func MustParse(s string) ID {
	return uuid.MustParse(s)
}

func IsNil(id ID) bool {
	return id == uuid.Nil
}
