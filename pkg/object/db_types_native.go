//go:build !js

package object

import "github.com/oarkflow/squealx"

// DB represents a database connection.
type DB struct {
	*squealx.DB
}

func (db *DB) Type() ObjectType { return DB_OBJ }
func (db *DB) Inspect() string  { return "<db connection>" }

// DBTx represents a database transaction.
type DBTx struct {
	*squealx.Tx
}

func (tx *DBTx) Type() ObjectType { return DB_TX_OBJ }
func (tx *DBTx) Inspect() string  { return "<db transaction>" }
