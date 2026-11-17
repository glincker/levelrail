# Install a catalog template by id, with its install form values and a domain.
resource "levelrail_template_app" "archive" {
  template = "archivebox"
  name     = "archive"
  env      = { ADMIN_USERNAME = "admin" }
  domains  = { archivebox = "archive.example.com" }
}
