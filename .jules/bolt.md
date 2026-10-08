## Already optimized
| Target | Date | Before | After |
| :--- | :--- | :--- | :--- |
| `internal/telemetry` WriteSamples | 2026-10-01 | 3810462 ns/op, 304707 B/op, 11012 allocs/op | 547262 ns/op, 150101 B/op, 23 allocs/op |
| `internal/telemetry` WriteLogBatch | 2026-10-01 | 47394281 ns/op, 448761 B/op, 13012 allocs/op | 23668267 ns/op, 286410 B/op, 22 allocs/op |
| `internal/store` ListDesiredServicesFiltered | 2026-09-29 | 344505 ns/op, 5529 B/op, 119 allocs/op | 343702 ns/op, 5529 B/op, 119 allocs/op |
| `internal/store` ListDeployments | 2026-09-29 | 2590872 ns/op, 240910 B/op, 1637 allocs/op | 2594280 ns/op, 240906 B/op, 1637 allocs/op |
| `internal/store` ListAppEvents | 2023-10-24 | 385868 ns/op | 384418 ns/op |
| `internal/store` ListDeployFreezeWindows | 2023-10-24 | 325603 ns/op | 312775 ns/op |
| `internal/store` UpsertConditions | 2026-10-05 | 2524228 ns/op, 25576 B/op, 757 allocs/op | 628789 ns/op, 10265 B/op, 21 allocs/op |
| `internal/compose` joinErrors | 2026-10-08 | 5779 ns/op, 7105 B/op, 10 allocs/op | 3369 ns/op, 4115 B/op, 3 allocs/op |

## Rejected

## Critical learnings
