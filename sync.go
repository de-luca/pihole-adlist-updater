package main

import (
    "log"
    "database/sql"
    "fmt"
    "os"
    "net/http"
    "encoding/csv"
    _ "github.com/mattn/go-sqlite3"
)

type Host struct {
    Category string
    Ticktype string
    SourceRepo string
    Description string
    SourceURL string
}

func (h Host) Comment() string {
    return fmt.Sprintf("[%s][%s] %s", h.Ticktype, h.Category, h.Description)
}

type Group struct {
    Name string
    Desc string
}

var listGroups = [...]Group{
    Group{
        Name: "tick",
        Desc: "Safe, least likely to interfere with browsing",
    },
    Group{
        Name: "std",
        Desc: "Standard",
    },
    Group{
        Name: "cross",
        Desc: "Dangerous, false postives, deprecated, biased",
    },
}

func rollbackAndQuit(tx *sql.Tx, err error) {
    log.Fatal(err)
    tx.Rollback()
    log.Printf("ROLLBACK TRANSACTION")
    os.Exit(1)
}

func logOrDie(function func(*sql.Tx) (int64, error), tx *sql.Tx, format string) {
    output, err := function(tx)
    if err != nil {
        rollbackAndQuit(tx, err)
    }
    log.Printf(format, output)
}


func fetchHosts() ([]Host, error) {
    res, err := http.Get("https://v.firebog.net/hosts/csv.txt")
    if err != nil {
        return nil, err
    }

    defer res.Body.Close()
    reader := csv.NewReader(res.Body)
    reader.Comma = ','

    lines, err := reader.ReadAll()
    if err != nil {
        return nil, err
    }

    var hosts []Host
    for _, line := range lines {
        hosts = append(hosts, Host{
            Category: line[0],
            Ticktype: line[1],
            SourceRepo: line[2],
            Description: line[3],
            SourceURL: line[4],
        })
    }

    return hosts, nil
}

func makeTmpTable(tx *sql.Tx, hosts []Host) (error) {
    tmpStmt := `
    CREATE TEMPORARY TABLE tmp_adlist (
        address text,
        enabled boolean,
        comment text
    );
    `
    _, err := tx.Exec(tmpStmt)
    if err != nil {
        return err
    }

    stmt, err := tx.Prepare("INSERT INTO tmp_adlist values(?, ?, ?)");
    if err != nil {
        return err
    }
    defer stmt.Close()

    for _, host := range hosts {
        _, err = stmt.Exec(host.SourceURL, true, host.Comment())
        if err != nil {
            return err
        }
    }

    return nil
}

func addMissing(tx *sql.Tx) (int64, error) {
    res, err := tx.Exec(`
    WITH missing AS (
        SELECT address, enabled, comment FROM tmp_adlist
        EXCEPT
        SELECT address, enabled, comment FROM adlist WHERE enabled AND comment LIKE '[%'
    )
    INSERT OR IGNORE INTO adlist (address, enabled, comment)
    SELECT * FROM missing;
    `)

    if err != nil {
        return 0, err
    }

    return res.RowsAffected()
}

func removeExtraneous(tx *sql.Tx) (int64, error) {
    res, err := tx.Exec(`
    WITH extraneous AS (
        SELECT address, enabled, comment FROM adlist WHERE enabled AND comment LIKE '[%'
        EXCEPT
        SELECT address, enabled, comment FROM tmp_adlist
    )
    UPDATE adlist
    SET enabled = false
    FROM extraneous
    WHERE adlist.address = extraneous.address;
    `)

    if err != nil {
        return 0, err
    }

    return res.RowsAffected()
}

func remapGroups(tx *sql.Tx) (int64, error) {
    _, err := tx.Exec(`
    DELETE FROM adlist_by_group 
    WHERE adlist_id IN (
        SELECT id FROM adlist WHERE comment LIKE '[%'
    );
    `)
    if err != nil {
        return 0, err
    }

    insertStmt, err := tx.Prepare(`
    INSERT OR IGNORE INTO 'group' (enabled, name, description)
    VALUES (?, ?, ?)
    `);
    if err != nil {
        return 0, err
    }
    defer insertStmt.Close()

    for _, group := range listGroups {
        _, err := insertStmt.Exec(true, group.Name, group.Desc)
        if err != nil {
            return 0, err
        }
    }

    res, err := tx.Exec(`
    INSERT OR IGNORE INTO adlist_by_group
    SELECT a.id, g.id
    FROM 'group' g
    JOIN adlist a ON a.comment LIKE '[' || g.name || '%'
    WHERE g.id != 0
    `)

    if err != nil {
        return 0, err
    }

    return res.RowsAffected()
}


func main() {
    db, err := sql.Open("sqlite3", "/pihole/gravity.db")
    if err != nil {
        log.Fatal(err)
        os.Exit(1)
    }
    defer db.Close()
    log.Printf("Gravity database opened")

    log.Printf("Fetching adlists from firebog")
    hosts, err := fetchHosts()
    if err != nil {
        log.Fatal(err)
        os.Exit(1)
    }
    log.Printf("Fetched %d adlist(s) from firebog", len(hosts))

    log.Printf("BEGIN TRANSACTION")
    tx, err := db.Begin()
    if err != nil {
        log.Fatal(err)
        os.Exit(1)
    }

    err = makeTmpTable(tx, hosts)
    if err != nil {
        rollbackAndQuit(tx, err)
    }

    logOrDie(addMissing, tx, "Added %d missing adlist(s)")
    logOrDie(removeExtraneous, tx, "Removed %d extraneous adlist(s)")
    logOrDie(remapGroups, tx, "Inserted %d adlist/group mapping(s)")
    
    tx.Commit()
    log.Printf("COMMIT TRANSACTION")
}