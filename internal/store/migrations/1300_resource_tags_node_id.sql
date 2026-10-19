UPDATE OR IGNORE resource_tags
SET name = (SELECT n.id FROM nodes n WHERE n.name = resource_tags.name)
WHERE kind = 'node'
  AND EXISTS (SELECT 1 FROM nodes n WHERE n.name = resource_tags.name);
