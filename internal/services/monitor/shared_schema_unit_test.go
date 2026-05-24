package monitor

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/stretchr/testify/require"
)

func TestMonitorMetricsAliasValidatorRequiresIdentifierAfterDollar(t *testing.T) {
	metricsAttr, ok := monitorMetricsAttribute().(rschema.ListNestedAttribute)
	require.True(t, ok, "metrics must be a ListNestedAttribute")

	aliasAttr, ok := metricsAttr.NestedObject.Attributes["alias"].(rschema.StringAttribute)
	require.True(t, ok, "alias must be a StringAttribute")
	require.NotEmpty(t, aliasAttr.Validators, "alias must validate the documented prefix")

	for _, value := range []string{"logs", "$", "$9logs", "$logs-hyphen"} {
		t.Run(value, func(t *testing.T) {
			var badResp validator.StringResponse
			for _, v := range aliasAttr.Validators {
				v.ValidateString(context.Background(), validator.StringRequest{
					Path:        path.Root("alias"),
					ConfigValue: types.StringValue(value),
				}, &badResp)
			}
			require.True(t, badResp.Diagnostics.HasError())
		})
	}

	var goodResp validator.StringResponse
	for _, v := range aliasAttr.Validators {
		v.ValidateString(context.Background(), validator.StringRequest{
			Path:        path.Root("alias"),
			ConfigValue: types.StringValue("$logs"),
		}, &goodResp)
	}
	require.False(t, goodResp.Diagnostics.HasError())
}
