use serde::Deserialize;
use csv::ReaderBuilder;

const URL: &str = "https://v.firebog.net/hosts/csv.txt";

#[derive(Debug, Deserialize)]
pub struct AdList {
    pub category: String,
    pub tick_type: String,
    pub source_repo: String,
    pub description: String,
    pub source_url: String,
}

impl AdList {
    pub fn get_comment(&self) -> String {
        format!("[{}][{}] {}", self.tick_type, self.category, self.description)
    }
}

pub async fn fetch() -> Result<Vec<AdList>, Box<dyn std::error::Error>> {
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
