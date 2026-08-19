package render

import "fmt"

// ExplainAPIs describes the two data sources this tool can read from.
//
// The distinction is the whole point of the participant-scoping rule: Scan is
// public indexed data, the Ledger API is an authorized participant projection.
// Neither is a global Canton transaction. Ports explain_apis.
func ExplainAPIs(scanUpdatePath, updateByIDPath string) string {
	return fmt.Sprintf(`Scan API vs Ledger API
----------------------
Scan API:
- Public/indexed network data from Super Validator Scan services.
- Useful for CantonScan-like flows and public update lookup.
- Endpoint used by dpm trace: GET %s
- Does not prove access to a bank/private participant projection.

Ledger JSON API:
- Authenticated participant/validator API.
- Requires participant URL, bearer token, and read-as/party context.
- Endpoint used by dpm trace: POST %s
- Returns the participant-visible projection; it is not a global trace.

In short:
- Scan is the public entry point.
- Ledger API is the authorized participant inspection entry point.`,
		scanUpdatePath, updateByIDPath)
}
