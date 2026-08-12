package ledger

import (
	"errors"

	"github.com/walnuthq/dpm-trace/internal/model"
)

// UpdateByIDBody builds the update-by-id request. Ports ledger_update_by_id_body.
//
// includeReassignments is set alongside the transaction shape so an unassign or
// assign update is returned rather than an empty response.
func UpdateByIDBody(updateID string, parties []string) map[string]any {
	filtersByParty := map[string]any{}
	for _, party := range parties {
		filtersByParty[party] = map[string]any{
			"cumulative": []any{
				map[string]any{
					"identifierFilter": map[string]any{
						"WildcardFilter": map[string]any{
							"value": map[string]any{"includeCreatedEventBlob": true},
						},
					},
				},
			},
		}
	}
	eventFormat := map[string]any{"filtersByParty": filtersByParty, "verbose": true}

	return map[string]any{
		"updateId": updateID,
		"updateFormat": map[string]any{
			"includeTransactions": map[string]any{
				"eventFormat":      eventFormat,
				"transactionShape": "TRANSACTION_SHAPE_LEDGER_EFFECTS",
			},
			"includeReassignments": eventFormat,
		},
	}
}

// LoadUpdate fetches an update, returning the raw response, the source label
// and the URL used. Ports load_update.
func (c *Client) LoadUpdate(updateID string, parties []string) (*model.Object, string, string, error) {
	if updateID == "" {
		return nil, "", "", errors.New("an update id or CantonScan update URL is required for Ledger API")
	}
	if c.BaseURL == "" {
		return nil, "", "", errors.New("--ledger-url is required for Ledger API fetches")
	}
	if len(parties) == 0 {
		return nil, "", "", errors.New("--read-as/--party is required for participant-scoped Ledger API fetches")
	}
	url := JoinURL(c.BaseURL, UpdateByIDPath)
	raw, err := c.JSON("POST", url, UpdateByIDBody(updateID, parties), true)
	return raw, "ledger-json-api", url, err
}

// LoadScanUpdate fetches an update from a Scan API. The result is public
// indexed data, not a participant projection, and must be labeled as such.
func (c *Client) LoadScanUpdate(updateID string) (*model.Object, string, string, error) {
	if updateID == "" {
		return nil, "", "", errors.New("an update id or CantonScan update URL is required for Scan API")
	}
	if c.BaseURL == "" {
		return nil, "", "", errors.New("--scan-url is required for Scan API fetches")
	}
	url := JoinURL(c.BaseURL, ScanUpdatePath+updateID)
	raw, err := c.JSON("GET", url, nil, true)
	return raw, "scan", url, err
}
