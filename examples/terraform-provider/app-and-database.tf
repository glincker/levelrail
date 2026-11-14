resource "levelrail_project" "shop" {
  name = "shop"
}

resource "levelrail_database" "main" {
  name       = "shop-db"
  engine     = "postgres"
  version    = "16"
  project_id = levelrail_project.shop.id

  pooler_enabled         = true
  pooler_pool_size       = 20
  pooler_max_client_conn = 200

  lifecycle {
    prevent_destroy = true
  }
}

resource "levelrail_app" "web" {
  name       = "web"
  image      = "nginx:1.27"
  port       = 80
  replicas   = 2
  strategy   = "rolling"
  project_id = levelrail_project.shop.id

  memory_mb = 256
  cpus      = 0.5
  restart   = "on-failure:3"

  readiness_path = "/"
  liveness_tcp   = true

  volumes = [{
    name           = "cache"
    container_path = "/var/cache/nginx"
  }]

  tags = ["team:shop"]

  labels = {
    team = "shop"
  }

  published_ports = [{
    container_port = 80
    host_port      = 8080
  }]
}

resource "levelrail_domain" "web" {
  app  = levelrail_app.web.name
  host = "shop.example.com"
}
