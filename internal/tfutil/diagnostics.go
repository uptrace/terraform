package tfutil

import (
	"github.com/hashicorp/terraform-plugin-framework/diag"

	"github.com/uptrace/terraform/internal/client"
)

// AddAPIError emits a generic API-error diagnostic. The backend's own message
// is surfaced as-is; callers don't need to special-case license-required or
// other server-owned error categories.
func AddAPIError(diags *diag.Diagnostics, summary string, err error) {
	diags.AddError(summary, client.APIErrorMessage(err))
}

// IsDeleteGone reports whether an API error means the resource is already
// gone. The backend returns 404 for absent or out-of-scope resources, so a
// 403 now unambiguously means action denied on an existing resource and
// should surface as an error rather than be swallowed here.
func IsDeleteGone(err error) bool {
	return client.IsNotFound(err)
}
