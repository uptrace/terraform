package org

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dsschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/stretchr/testify/require"
)

func TestOrgUserDataSource_emailRejectsMixedCase(t *testing.T) {
	var schemaResp datasource.SchemaResponse
	(&OrgUserDataSource{}).Schema(context.Background(), datasource.SchemaRequest{}, &schemaResp)
	require.False(t, schemaResp.Diagnostics.HasError())

	emailAttr, ok := schemaResp.Schema.Attributes["email"].(dsschema.StringAttribute)
	require.True(t, ok)

	var resp validator.StringResponse
	for _, v := range emailAttr.Validators {
		v.ValidateString(context.Background(), validator.StringRequest{
			Path:        path.Root("email"),
			ConfigValue: types.StringValue("Foo@Bar.com"),
		}, &resp)
	}

	require.True(t, resp.Diagnostics.HasError())
	require.Contains(t, resp.Diagnostics.Errors()[0].Summary(), "email must be lowercase and trimmed")
}
