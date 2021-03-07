use crate::adlists::AdList;
use rusqlite::{Transaction, NO_PARAMS};

#[derive(Debug)]
struct Group<'a> {
    name: &'a str,
    desc: &'a str,
}

const GROUPS: [Group; 3] = [
    Group {
        name: "tick",
        desc: "Safe, least likely to interfere with browsing",
    },
    Group {
        name: "std",
        desc: "Standard",
    },
    Group {
        name: "cross",
        desc: "Dangerous, false postives, deprecated, biased",
    },
];

pub fn make_tmp_table(
    tx: &Transaction,
    lists: &Vec<AdList>,
) -> Result<(), Box<dyn std::error::Error>> {
    let _ = tx.execute("
        CREATE TEMPORARY TABLE tmp_adlist (
            address text,
            enabled boolean,
            comment text
        )
    ", NO_PARAMS)?;

    let mut stmt = tx.prepare("INSERT INTO tmp_adlist values(?, ?, ?)")?;
    for list in lists {
        let _ = stmt.insert(&[&list.source_url, "true", &list.get_comment()]);
    }

    Ok(())
}

pub fn add_missing(tx: &Transaction) -> Result<usize, Box<dyn std::error::Error>> {
    let inserted = tx.execute("
        WITH missing AS (
            SELECT address, enabled, comment FROM tmp_adlist
            EXCEPT
            SELECT address, enabled, comment FROM adlist WHERE enabled AND comment LIKE '[%'
        )
        INSERT OR IGNORE INTO adlist (address, enabled, comment)
        SELECT * FROM missing
    ", NO_PARAMS)?;

    Ok(inserted)
}

pub fn remove_extraneous(tx: &Transaction) -> Result<usize, Box<dyn std::error::Error>> {
    let removed = tx.execute("
        UPDATE adlist
        SET enabled = true
        WHERE enabled
        AND comment LIKE '[%'
        AND comment NOT IN (SELECT comment FROM tmp_adlist)
    ", NO_PARAMS)?;

    Ok(removed)
}

pub fn remap_groups(tx: &Transaction) -> Result<usize, Box<dyn std::error::Error>> {
    let _ = tx.execute("
        DELETE FROM adlist_by_group
        WHERE adlist_id IN (
            SELECT id FROM adlist WHERE comment LIKE '[%'
        )
    ", NO_PARAMS);

    let mut stmt = tx.prepare("
        INSERT OR IGNORE INTO 'group' (enabled, name, description)
        VALUES (?, ?, ?)
    ")?;

    for group in &GROUPS {
        let _ = stmt.insert(&["true", group.name, group.desc]);
    }

    let mapped = tx.execute("
        INSERT OR IGNORE INTO adlist_by_group
        SELECT a.id, g.id
        FROM 'group' g
        JOIN adlist a ON a.comment LIKE '[' || g.name || '%'
        WHERE g.id != 0
    ", NO_PARAMS)?;

    Ok(mapped)
}
