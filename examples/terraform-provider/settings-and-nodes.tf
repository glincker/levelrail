# Keep the last 10 inline sources; destroying this resets the setting to its default.
resource "levelrail_setting" "inline_keep" {
  key   = "deploy.inline_source_keep"
  value = "10"
}

# Labels and taints on a node that already exists. Other labels stay untouched.
resource "levelrail_node_labels" "edge" {
  node = "edge-1"

  labels = {
    zone = "eu-west"
    tier = "edge"
  }

  taints = [
    { key = "dedicated", value = "gpu", effect = "NoSchedule" },
  ]
}

data "levelrail_nodes" "all" {}
