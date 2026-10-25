-- Only the newest succeeded attempt of an app may claim rollout_state serving.
UPDATE deploy_attempts SET rollout_state = 'superseded'
WHERE rollout_state = 'serving' AND EXISTS (
    SELECT 1 FROM deploy_attempts n
    WHERE n.service_name = deploy_attempts.service_name AND n.rollout_state = 'serving'
      AND n.started_at > deploy_attempts.started_at
);
