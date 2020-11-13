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
    
    tx.Commit()


    // rows, err := db.Query("SELECT * FROM adlist WHERE comment LIKE '[%'")
    // if err != nil {
    // 	panic(err)
    // }
    // defer rows.Close()
    // for rows.Next() {
    // 	var id int
    // 	var address string
    // 	err = rows.Scan(&id, &address)
    // 	if err != nil {
    // 		panic(err)
    // 	}
    // 	fmt.Println(id, address)
    // }
    // err = rows.Err()
}