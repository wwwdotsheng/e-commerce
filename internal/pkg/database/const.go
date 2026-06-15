package database

type LockType string

const (
	LockUpdate LockType = "UPDATE"
	LockShare  LockType = "SHARE"
	LockNone   LockType = "NONE"
)
