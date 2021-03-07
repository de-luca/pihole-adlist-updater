mod adlists;
mod db;

use clap::Clap;
use rusqlite::{Connection, Transaction};

/// This doc string acts as a help message when the user runs '--help'
/// as do all doc strings on fields
#[derive(Clap)]
#[clap()]
struct Opts {
    /// The gravity.db file path
    #[clap(short, long, default_value = "/etc/pihole/gravity.db")]
    db: String,
}

async fn run<'a>(tx: &Transaction<'a>) -> Result<(), Box<dyn std::error::Error>> {
    println!("Fetching adlists from firebog");
    let lists = adlists::fetch().await?;
    println!("Fetched {} adlist(s)", lists.len());

    let _ = db::make_tmp_table(&tx, &lists)?;
    let added = db:: add_missing(&tx)?;
    println!("Added {} missing adlist(s)", added);
    let removed = db::remove_extraneous(&tx)?;
    println!("Removed {} extraneous adlist(s)", removed);
    let mapped = db::remap_groups(&tx)?;
    println!("Inserted {} adlist/group mapping(s)", mapped);

    Ok(())
}

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    let opts: Opts = Opts::parse();

    if !std::path::Path::new(&opts.db).exists() {
        Err(format!("DB path '{}' is invalid.", opts.db))?
    }

    let mut conn = Connection::open(&opts.db)?;
    let tx = conn.transaction()?;
    println!("Opened DB: '{}'.", &opts.db);

    match run(&tx).await {
        Ok(_) => {
            tx.commit().expect("Could not commit transaction.");
            Ok(())
        }
        Err(err) => {
            println!("{}", err);
            tx.rollback().expect("Cound not rollback transaction.");
            Err(err)
        }
    }
}
