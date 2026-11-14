resource "levelrail_environment" "staging" {
  project_id = levelrail_project.shop.id
  name       = "staging"
  protected  = false
}

resource "levelrail_environment_variables" "shop" {
  scope    = "project"
  scope_id = levelrail_project.shop.id

  variables = {
    LOG_LEVEL = "info"
  }
}

resource "levelrail_app_secret" "session" {
  app   = levelrail_app.web.name
  key   = "SESSION_KEY"
  value = var.session_key
}

resource "levelrail_alert_rule" "web_crashloop" {
  app                     = levelrail_app.web.name
  name                    = "web crash loop"
  kind                    = "crashloop"
  restart_count_threshold = 3
}

data "levelrail_apps" "all" {}

data "levelrail_app_secrets" "web" {
  app = levelrail_app.web.name
}
