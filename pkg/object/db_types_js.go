//go:build js

package object

// DB is a js/wasm stub that preserves the public object surface without
// linking any native database drivers into browser builds.
type DB struct{}

func (db *DB) Type() ObjectType { return DB_OBJ }
func (db *DB) Inspect() string  { return "<db unavailable in js/wasm>" }

// DBTx is a js/wasm stub that preserves the public object surface without
// linking any native database drivers into browser builds.
type DBTx struct{}

func (tx *DBTx) Type() ObjectType { return DB_TX_OBJ }
func (tx *DBTx) Inspect() string  { return "<db transaction unavailable in js/wasm>" }
