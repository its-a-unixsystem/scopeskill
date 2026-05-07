package scopeskill

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// KontaktMasterFields are the master-directory fields exposed by stitched
// `kontakt` blocks: identifying name, address, contact info, USt-ID, plus the
// link numbers to the Debitor/Kreditor accounts. Reused by slice 5
// (debitor|kreditor show stitching) and slice 6 (offene-posten show
// stitching).
var KontaktMasterFields = []string{
	"id",
	"lastname",
	"companyname",
	"firstname",
	"email",
	"phone",
	"vatId",
	"taxId",
	"street1",
	"postcode1",
	"city1",
	"country1",
	"debitorNumber",
	"kreditorNumber",
}

// FetchKontaktByID returns the Kontakt with the given id, projected to
// KontaktMasterFields. Returns nil when the contact is not found
// (Scopevisio reports this as HTTP 400 per the live Swagger).
//
// Reused by slice 5 (debitor|kreditor show) and slice 6 (offene-posten show)
// for the `kontakt` block of their stitched responses.
func FetchKontaktByID(client *Client, id any) (map[string]any, error) {
	if id == nil || id == "" {
		return nil, errors.New("FetchKontaktByID: id is required")
	}
	path := fmt.Sprintf("/contact/%v", id)
	raw, err := client.JSON(http.MethodGet, path, nil, map[string]string{
		"fields": strings.Join(KontaktMasterFields, ","),
	})
	if err != nil {
		var apiErr APIError
		if errors.As(err, &apiErr) && (apiErr.StatusCode == http.StatusNotFound || apiErr.StatusCode == http.StatusBadRequest) {
			return nil, nil
		}
		return nil, err
	}
	obj, ok := raw.(map[string]any)
	if !ok {
		return nil, nil
	}
	return obj, nil
}
