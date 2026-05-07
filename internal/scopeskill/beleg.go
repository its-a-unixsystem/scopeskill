package scopeskill

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
)

const (
	BelegEndpointIncomingInvoice = "/incominginvoice"
	BelegEndpointOutgoingInvoice = "/outgoinginvoice"
	BelegEndpointCredit          = "/credit"
)

// FetchBeleg returns a document-like Beleg from one of the invoice/credit
// endpoints, or nil when Scopevisio reports it missing.
func FetchBeleg(client *Client, endpoint string, number string) (map[string]any, error) {
	if endpoint == "" {
		return nil, errors.New("FetchBeleg: endpoint is required")
	}
	if number == "" {
		return nil, errors.New("FetchBeleg: number is required")
	}
	path := fmt.Sprintf("%s/%s", endpoint, url.PathEscape(number))
	raw, err := client.JSON(http.MethodGet, path, nil, nil)
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
