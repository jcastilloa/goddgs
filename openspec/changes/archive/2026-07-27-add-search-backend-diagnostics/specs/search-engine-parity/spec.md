## ADDED Requirements

### Requirement: Diagnostic extension does not change source orchestration
The optional Go diagnostic extension SHALL be observational and SHALL NOT change frozen backend selection, provider de-duplication timing, worker count, aggregation, ranking, raw result fields, errors, or cancellation behavior.

#### Scenario: Diagnostics enabled for provider-colliding engines
- **WHEN** diagnostics are enabled for a controlled search containing engines that share a provider
- **THEN** selected and aggregated results SHALL match the same source-compatible scheduler behavior as when diagnostics are disabled
