package tfutil

import (
	"github.com/hashicorp/terraform-plugin-framework/diag"

	"github.com/uptrace/terraform/internal/client"
)

// AddLicenseRequiredError emits a diagnostic for a license-required error.
// Call only after client.IsLicenseRequired confirms the error type.
func AddLicenseRequiredError(diags *diag.Diagnostics, err error) {
	diags.AddError(
		"Uptrace license required",
		client.APIErrorMessage(err)+
			". Install a license key in the backend config, or build the backend with -tags billing.",
	)
}

// AddAPIError emits a generic API-error diagnostic. Callers should handle
// special cases (license-required, not-found) before calling it.
func AddAPIError(diags *diag.Diagnostics, summary string, err error) {
	diags.AddError(summary, client.APIErrorMessage(err))
}

// IsDeleteGone reports whether a Delete-time error means the resource is
// already gone. License-required 403s are excluded so a backend downgrade
// surfaces instead of being swallowed.
func IsDeleteGone(err error) bool {
	if client.IsNotFound(err) {
		return true
	}
	return client.IsForbidden(err) && !client.IsLicenseRequired(err)
}
