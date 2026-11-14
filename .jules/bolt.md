## Already optimized
| Target | Date | Before | After |
| :--- | :--- | :--- | :--- |
| `internal/store/deployment_list.go` `EXPLAIN` | 2026-10-15 | `json_each` table scan | `IN` index seek |

## Rejected
- `internal/store/reconcile_status.go` `GetConditionsForControllers` `json_each`: tested replacing with `IN (...)`. The `json_each` is actually faster and allocates less memory (261µs, 883 allocs vs 286µs, 1036 allocs), despite the query planner indicating a virtual table scan.
## Skip streak
- 2026-11-09 Monday rotation (internal/store/**): json_each vs IN in GetConditionsForControllers proved json_each is faster and allocates less, contrary to EXPLAIN QUERY PLAN concerns.
