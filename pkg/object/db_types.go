package object

// DB represents an optional database connection value. The concrete Handle is
// owned by an opt-in database builtin package so the core object package does
// not depend on any database driver module.
type DB struct {
	Handle any
}

func (db *DB) Type() ObjectType { return DB_OBJ }
func (db *DB) Inspect() string  { return "<db connection>" }

// DBTx represents an optional database transaction value. The concrete Handle
// is owned by an opt-in database builtin package.
type DBTx struct {
	Handle any
}

func (tx *DBTx) Type() ObjectType { return DB_TX_OBJ }
func (tx *DBTx) Inspect() string  { return "<db transaction>" }
