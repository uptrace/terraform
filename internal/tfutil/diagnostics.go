package tfutil

import (
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"

	"github.com/uptrace/terraform/internal/client"
)

// AddAPIError emits a generic API-error diagnostic. The backend's own message
// is surfaced as-is; callers don't need to special-case license-required or
// other server-owned error categories.
func AddAPIError(diags *diag.Diagnostics, summary string, err error) {
	diags.AddError(summary, client.APIErrorMessage(err))
}

// IsDeleteGone reports whether a Delete-time error means the resource is
// already gone. A 403 whose server message mentions "license" is excluded so
// a backend downgrade surfaces instead of being swallowed.
func IsDeleteGone(err error) bool {
	if client.IsNotFound(err) {
		return true
	}
	if !client.IsForbidden(err) {
		return false
	}
	return !strings.Contains(strings.ToLower(client.APIErrorMessage(err)), "license")
}
