// Package `db` manages the user and roles database.
package db

import (
	"database/sql"
	"fmt"
	"sync"
	"time"

	_ "github.com/ncruces/go-sqlite3/driver"
	_ "github.com/ncruces/go-sqlite3/embed"
	"golang.org/x/crypto/bcrypt"
)

// The version of the database, used for migrations.
// Will stay at 0 until I stop introducing breaking changes constantly.
const version int = 0

// Represents a connection to the database. Used for database operations.
type Database struct {
	db *sql.DB
    mu sync.Mutex
}

type Ban struct {
	BanID     int
	IPID      string
	HDID      string
	Reason    string
	Moderator string
	Start     time.Time
	End       time.Time
}

// Opens a connection to the database, creating it and initializing the tables if necessary.
func Init(path string) (*Database, error) {
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, fmt.Errorf("db: Couldn't connect to database (%w).", err)
	}

	// TODO: users table?

	_, err = db.Exec(`
    CREATE TABLE IF NOT EXISTS auth(
        username TEXT PRIMARY KEY,
        password TEXT NOT NULL,
        role     TEXT NOT NULL
    )`)
	if err != nil {
		return nil, fmt.Errorf("db: Couldn't create auth table (%w).", err)
	}

	_, err = db.Exec(`
    CREATE TABLE IF NOT EXISTS bans(
        ban_id    INTEGER PRIMARY KEY,
        ipid      TEXT,
        hdid      TEXT,
        reason    TEXT NOT NULL,
        moderator TEXT NOT NULL,
        start     INTEGER NOT NULL,
        end       INTEGER NOT NULL,

        CHECK (ipid IS NOT NULL OR hdid IS NOT NULL)
    )`)
	if err != nil {
		return nil, fmt.Errorf("db: Couldn't create bans table (%w).", err)
	}

	return &Database{db: db}, nil
}

// Adds a new ban to the database.
func (d *Database) AddBan(ipid string, hdid string, reason string, moderator string, duration time.Duration) error {
	// Get time right away.
	start := time.Now()
	end := start.Add(duration)

    if ipid != "" && hdid != "" {
        _, err := d.db.Exec(`
        INSERT INTO bans
            (ipid, hdid, reason, moderator, start, end)
        VALUES
            (?, ?, ?, ?, ?, ?)`,
            ipid, hdid, reason, moderator, start.Unix(), end.Unix())
        if err != nil {
            return fmt.Errorf("db: Couldn't insert ban (%w).", err)
        }
        return nil
    }

    var id string
    var st *sql.Stmt
    var err error
    switch {
    case ipid == "":
        id = hdid
        st, err = d.db.Prepare(`
        INSERT INTO bans
            (ipid, hdid, reason, moderator, start, end)
        VALUES
            (NULL, ?, ?, ?, ?, ?)`)
        if err != nil {
            return fmt.Errorf("db: Couldn't insert HDID ban (%w).", err)
        }

    case hdid == "":
        id = ipid
        st, err = d.db.Prepare(`
        INSERT INTO bans
            (ipid, hdid, reason, moderator, start, end)
        VALUES
            (?, NULL, ?, ?, ?, ?)`)
        if err != nil {
            return fmt.Errorf("db: Couldn't insert IPID ban (%w).", err)
        }
    default:
		return fmt.Errorf("db: IPID and HDID cannot both be empty.")
    }

    if _, err := st.Exec(id, reason, moderator, start.Unix(), end.Unix()); err != nil {
        return fmt.Errorf("db: Couldn't insert ban (%w).", err)
    }
	return nil
}

// Gets all bans that correspond to the passed IPID and HDID (including expired ones).
func (d *Database) GetBans(ipid string, hdid string) ([]Ban, error) {
	var bans []Ban

    rows, err := d.db.Query("SELECT DISTINCT * FROM bans WHERE ipid = ? OR hdid = ?", ipid, hdid)
    if err != nil {
        return nil, fmt.Errorf("db: Couldn't query database (%w).", err)
    }
    for rows.Next() {
        var ban Ban
        var ipid sql.NullString
        var hdid sql.NullString
        var start int64
        var end int64
        if err := rows.Scan(&ban.BanID, &ipid, &hdid, &ban.Reason, &ban.Moderator, &start, &end); err != nil {
            return bans, fmt.Errorf("db: Error scanning row (%w).", err)
        }
        ban.IPID = ipid.String
        ban.HDID = hdid.String
        ban.Start = time.Unix(start, 0)
        ban.End = time.Unix(end, 0)
        bans = append(bans, ban)
    }
	return bans, nil
}

// Verify if a given IPID and HDID is banned. If either are a match, returns a list of
// non-expired bans on this user.
func (d *Database) CheckBanned(ipid string, hdid string) (bool, []Ban, error) {
    bans, err := d.GetBans(ipid, hdid)
    if err != nil {
        return false, bans, err
    }

    banned := false
    validBans := make([]Ban, 0, len(bans))
    for _, ban := range bans {
        if time.Now().Before(ban.End) {
            banned = true
            validBans = append(validBans, ban)
        }
    }
    return banned, validBans, nil
}

// Nullifies a ban by setting its end time to the current time.
func (d *Database) NullBan(id int) error {
	now := time.Now().Unix()
	_, err := d.db.Exec(`
    UPDATE bans
    SET end = ?
    WHERE ban_id = ?`,
		now, id)
	if err != nil {
		return fmt.Errorf("db: Couldn't null ban (%w).", err)
	}
	return nil
}

// Nullifies all bans for the passed IPID and HDID.
func (d *Database) NullBans(ipid string, hdid string) error {
	bans, err := d.GetBans(ipid, hdid)
	if err != nil {
		return fmt.Errorf("db: Couldn't get bans (%w).", err)
	}
	for _, ban := range bans {
		if err := d.NullBan(ban.BanID); err != nil {
			return fmt.Errorf("db: Couldn't null ban of ID %v (%w).", ban.BanID, err)
		}
	}
	return nil
}

// Adds a new user that can authenticate to the passed role.
func (d *Database) AddAuth(username string, password string, role string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("db: Error hashing password (%w).", err)
	}
	_, err = d.db.Exec(`
    INSERT INTO auth
        (username, password, role)
    VALUES
        (?, ?, ?)`,
		username, string(hash), role)
	if err != nil {
		return fmt.Errorf("db: Couldn't add user (%w).", err)
	}
	return nil
}

// Checks whether a given username and password authenticate to a user.
func (d *Database) CheckAuth(username string, password string) (ok bool, role string, err error) {
    row := d.db.QueryRow("SELECT password, role FROM auth WHERE username = ?", username)
    if row.Err() == sql.ErrNoRows {
        // user doesn't exist
        return false, "", nil
    }
    var hash string
    // var role string
    if err := row.Scan(&hash, &role); err != nil {
        return false, "", err
    }
    if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
        return false, "", nil
    }
    return true, role, nil
}

// Removes a user from the auth table.
func (d *Database) RemoveAuth(username string) error {
	if _, err := d.db.Exec("DELETE FROM auth WHERE username = ?", username); err != nil {
		return fmt.Errorf("db: Couldn't remove user (%w).", err)
	}
	return nil
}

// Closes the database connection.
func (d *Database) Close() error {
    if err := d.db.Close(); err != nil {
        return fmt.Errorf("db: Error closing database (%w).", err)
    }
    return nil
}
