# A zone at the connected DNS provider and a record in it.
resource "levelrail_dns_zone" "main" {
  name = "example.com"
}

resource "levelrail_dns_record" "www" {
  zone   = levelrail_dns_zone.main.name
  name   = "www"
  type   = "A"
  ttl    = 300
  values = ["203.0.113.10"]
}

# Read the template catalog and the newest successful deploy of an app.
data "levelrail_templates" "databases" {
  category = "database"
}

data "levelrail_latest_deploy" "web" {
  app    = "shop-web"
  status = "succeeded"
}
