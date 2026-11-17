// Package gmdb reads characters' access levels from the game server's
// MariaDB for the bots' GM authority (docs/plan.md, "In-game control";
// docs/gm-control.md). The data source name is a secret: it is never
// logged, and errors never carry it.
package gmdb

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
)

// Connection limits. maxConns is the database user's MAX_USER_CONNECTIONS
// (docs/gm-control.md); the bots' authority runs as many checks at once.
const (
	maxConns    = 2
	dialTimeout = 5 * time.Second
	ioTimeout   = 5 * time.Second
	idleFor     = time.Minute
)

// query is the one statement the bots run: uncached, per command.
const query = "SELECT accesslevel, deletetime FROM characters WHERE obj_Id = ?"

// errBadDSN reports a data source name the driver cannot parse, without
// repeating it.
var errBadDSN = errors.New("gm database: malformed data source name")

// Levels reads access levels; it implements botclient.LevelLookup. It is
// safe for concurrent use.
type Levels struct {
	db *sql.DB
	// secrets are the strings redact removes from errors.
	secrets []string
}

// Open returns the access levels behind dsn (go-sql-driver/mysql format)
// on a pool of at most maxConns connections. It dials nothing yet.
func Open(dsn string) (*Levels, error) {
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		return nil, errBadDSN
	}
	cfg.Timeout, cfg.ReadTimeout, cfg.WriteTimeout = dialTimeout, ioTimeout, ioTimeout
	// The driver's own logger writes to stderr; the bots report through
	// the errors they get.
	cfg.Logger = &mysql.NopLogger{}
	conn, err := mysql.NewConnector(cfg)
	if err != nil {
		return nil, errBadDSN
	}
	db := sql.OpenDB(conn)
	db.SetMaxOpenConns(maxConns)
	db.SetMaxIdleConns(maxConns)
	db.SetConnMaxIdleTime(idleFor)
	l := &Levels{db: db, secrets: []string{dsn}}
	if cfg.Passwd != "" {
		l.secrets = append(l.secrets, cfg.Passwd)
	}
	return l, nil
}

// AccessLevel reads the access level of the character objectID and whether
// it is pending deletion. A character the table lacks reads as level 0, not
// deleted; a NULL level reads as 0, as the column's default.
func (l *Levels) AccessLevel(ctx context.Context, objectID int32) (level int32, deleted bool, err error) {
	var lv sql.NullInt32
	var del sql.NullInt64
	err = l.db.QueryRowContext(ctx, query, objectID).Scan(&lv, &del)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, redact(err, l.secrets)
	}
	return lv.Int32, del.Int64 != 0, nil
}

// Close closes the pool.
func (l *Levels) Close() error {
	return redact(l.db.Close(), l.secrets)
}

// redact returns err with every secret cut from its text, or err itself
// when its text holds none.
func redact(err error, secrets []string) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	clean := msg
	for _, s := range secrets {
		clean = strings.ReplaceAll(clean, s, "[redacted]")
	}
	if clean == msg {
		return err
	}
	return errors.New(clean)
}
