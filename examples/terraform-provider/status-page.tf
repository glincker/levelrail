resource "levelrail_status_page" "main" {
  enabled       = true
  title         = "Acme status"
  description   = "Live health of Acme services."
  custom_domain = "status.example.com"
}

resource "levelrail_status_component" "web" {
  kind   = "app"
  target = levelrail_app.web.name
  name   = "Web"
  order  = 1
}

resource "levelrail_status_component" "site" {
  kind   = "domain"
  target = "shop.example.com"
  name   = "Storefront"
  order  = 2
}
