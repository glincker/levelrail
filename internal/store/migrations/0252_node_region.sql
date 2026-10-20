-- Optional operator-facing location label for a node (e.g. "hetzner-fsn1",
-- "aws-us-east-1", "home-lab"): display/grouping metadata for the network
-- topology view only, not a routing or access-control mechanism. Free
-- text, no fixed provider/region list to validate against.
ALTER TABLE nodes ADD COLUMN region TEXT NOT NULL DEFAULT '';
