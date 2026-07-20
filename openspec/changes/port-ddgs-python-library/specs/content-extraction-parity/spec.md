## ADDED Requirements

### Requirement: Source-compatible URL fetch for extraction
Extract operation SHALL fetch caller URL using client proxy, timeout, and TLS
configuration, return DDGS-classifiable error for non-200 source status, and
close transport resources on success, failure, or cancellation.

#### Scenario: Successful source response
- **WHEN** fixture transport returns HTTP 200 for extract URL
- **THEN** extract operation SHALL return original requested URL and source-compatible selected content

#### Scenario: Non-200 source response
- **WHEN** fixture transport returns non-200 status for extract URL
- **THEN** extract operation SHALL return DDGS-classifiable failure identifying URL and HTTP status

#### Scenario: Extraction context is canceled
- **WHEN** caller cancels context while extract request is in progress
- **THEN** operation SHALL stop request work, return context-classifiable error, and release response resources

### Requirement: Extraction format parity
Extract operation SHALL support frozen source formats `text_markdown`,
`text_plain`, `text_rich`, `text`, and `content`. It SHALL return raw response
text for `text`, raw bytes for `content`, source-compatible rendered output for
Markdown/plain/rich, and Markdown fallback for unknown format.

#### Scenario: Raw text format is requested
- **WHEN** caller requests `text`
- **THEN** returned content SHALL equal source response text without renderer transformation

#### Scenario: Raw byte format is requested
- **WHEN** caller requests `content`
- **THEN** returned content SHALL equal source response bytes

#### Scenario: Rendered formats are requested
- **WHEN** fixtures request Markdown, plain, and rich output from representative HTML pages
- **THEN** each Go output SHALL match corresponding frozen Python fixture output

#### Scenario: Unknown format is requested
- **WHEN** caller supplies format not recognized by frozen source branching
- **THEN** returned content SHALL match source Markdown fallback output

### Requirement: Rendered-output evidence gate
Rendered Markdown, plain-text, and rich-text implementation SHALL be selected
only after differential fixture corpus proves it against frozen Python output.
Generic renderer SHALL NOT be substituted merely because it produces readable
output.

#### Scenario: Renderer candidate differs from source fixture
- **WHEN** candidate renderer produces output different from frozen source for required fixture
- **THEN** it SHALL not be accepted until compatibility adapter or explicit scope decision resolves difference
