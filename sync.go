package main

import (
    "database/sql"
    "fmt"
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

func makeTmpTable(tx *sql.Tx, hosts []Host) {
    tmpStmt := `
    CREATE TEMPORARY TABLE tmp_adlist (
        address text,
        enabled boolean,
        comment text
    );
    `
    _, err := tx.Exec(tmpStmt)
    if err != nil {
        panic(err)
    }

    stmt, err := tx.Prepare("INSERT INTO tmp_adlist values(?, ?, ?)");
    if err != nil {
        panic(err)
    }
    defer stmt.Close()
    for _, host := range hosts {
        _, err = stmt.Exec(host.SourceURL, true, host.Comment())
        if err != nil {
            panic(err)
        }
    }
}

func addMissing(tx *sql.Tx) {
    missingStmt := `
    WITH missing AS (
        SELECT address, enabled, comment FROM tmp_adlist
        EXCEPT
        SELECT address, enabled, comment FROM adlist WHERE enabled AND comment LIKE '[%'
    )
    INSERT OR IGNORE INTO adlist (address, enabled, comment)
    SELECT * FROM missing;
    `

    _, err := tx.Exec(missingStmt)
    if err != nil {
        panic(err)
    }
}

func removeExtraneous(tx *sql.Tx) {
    extraStmt := `
    WITH extraneous AS (
        SELECT address, enabled, comment FROM adlist WHERE enabled AND comment LIKE '[%'
        EXCEPT
        SELECT address, enabled, comment FROM tmp_adlist
    )
    UPDATE adlist
    SET enabled = false
    FROM extraneous
    WHERE adlist.address = extraneous.address;
    `

    _, err := tx.Exec(extraStmt)
    if err != nil {
        panic(err)
    }
}


func remapGroups(tx *sql.Tx) {
    _, err := tx.Exec(`
    DELETE FROM adlist_by_group 
    WHERE adlist_id IN (
        SELECT id FROM adlist WHERE comment LIKE '[%'
    );
    `)

    insertStmt, _ := tx.Prepare(`
    INSERT OR IGNORE INTO 'group' (enabled, name, description)
    VALUES (?, ?, ?)
    `);
    defer insertStmt.Close()

    for _, group := range listGroups {
        _, err := insertStmt.Exec(true, group.Name, group.Desc)
        if err != nil {
            panic(err)
        }
    }

    mapStmt := `
    INSERT OR IGNORE INTO adlist_by_group
    SELECT a.id, g.id
    FROM 'group' g
    JOIN adlist a ON a.comment LIKE '[' || g.name || '%'
    WHERE g.id != 0
    `

    _, err = tx.Exec(mapStmt)
    if err != nil {
        panic(err)
    }
}


func main() {
    db, err := sql.Open("sqlite3", "/pihole/gravity.db")
    if err != nil {
        panic(err)
    }
    defer db.Close()
    tx, err := db.Begin()
    if err != nil {
        panic(err)
    }

    hosts, err := fetchHosts()
    if err != nil {
        panic(err)
    }

    makeTmpTable(tx, hosts)
    addMissing(tx)
    removeExtraneous(tx)
    remapGroups(tx)
    
    tx.Commit()
}