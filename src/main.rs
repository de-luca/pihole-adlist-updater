use clap::Clap;
use serde::Deserialize;
use csv::ReaderBuilder;
use rusqlite::{Connection, Transaction, NO_PARAMS};

/// This doc string acts as a help message when the user runs '--help'
/// as do all doc strings on fields
#[derive(Clap)]
#[clap()]
struct Opts {
    /// The gravity.db file path
    #[clap(short, long, default_value = "/etc/pihole/gravity.db")]
    db: String,
}

#[derive(Debug, Deserialize)]
struct AdList {
    category: String,
    tick_type: String,
    source_repo: String,
    description: String,
    source_url: String,
}

impl AdList {
    fn get_comment(&self) -> String {
        format!("[{}][{}] {}", self.tick_type, self.category, self.description)
    }
}

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

const URL: &str = "https://v.firebog.net/hosts/csv.txt";

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    let opts: Opts = Opts::parse();

    if !std::path::Path::new(&opts.db).exists() {
        Err(format!("DB path '{}' is invalid.", opts.db))?
    }

    let mut conn = Connection::open(&opts.db)?;
    let tx = conn.transaction()?;
    println!("Opened DB: '{}'.", &opts.db);

    println!("Fetching adlists from firebog");
    let lists = fetch_adlists().await?;
    println!("Fetched {} adlist(s)", lists.len());

    let _ = make_tmp_table(&tx, &lists)?;
    let added = add_missing(&tx)?;
    println!("Added {} missing adlist(s)", added);
    let removed = remove_extraneous(&tx)?;
    println!("Removed {} extraneous adlist(s)", removed);
    let mapped = remap_groups(&tx)?;
    println!("Inserted {} adlist/group mapping(s)", mapped);

    let _ = tx.commit();

    Ok(())
}

async fn fetch_adlists() -> Result<Vec<AdList>, Box<dyn std::error::Error>> {
    let resp = reqwest::get(URL)
        .await?
        .text()
        .await?;

    let mut rdr = ReaderBuilder::new()
        .delimiter(b',')
        .has_headers(false)
        .quote(b'"')
        .from_reader(resp.as_bytes());

    let mut list: Vec<AdList> = vec![];
    for result in rdr.deserialize() {
        list.push(result?);
    }

    Ok(list)
}

fn make_tmp_table(
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

fn add_missing(tx: &Transaction) -> Result<usize, Box<dyn std::error::Error>> {
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

fn remove_extraneous(tx: &Transaction) -> Result<usize, Box<dyn std::error::Error>> {
    let removed = tx.execute("
        UPDATE adlist
        SET enabled = true
        WHERE enabled
        AND comment LIKE '[%'
        AND comment NOT IN (SELECT comment FROM tmp_adlist)
    ", NO_PARAMS)?;

    Ok(removed)
}

fn remap_groups(tx: &Transaction) -> Result<usize, Box<dyn std::error::Error>> {
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
