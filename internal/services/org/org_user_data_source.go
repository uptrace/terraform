package org

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/uptrace/terraform/internal/client"
	"github.com/uptrace/terraform/internal/generated"
	"github.com/uptrace/terraform/internal/tfutil"
)

var (
	_ datasource.DataSource              = &OrgUserDataSource{}
	_ datasource.DataSourceWithConfigure = &OrgUserDataSource{}
)

type OrgUserDataSource struct {
	client *client.Client
}

type orgUserDataSourceModel struct {
	ID     types.String `tfsdk:"id"`
	OrgID  types.String `tfsdk:"org_id"`
	Email  types.String `tfsdk:"email"`
	UserID types.String `tfsdk:"user_id"`
	Role   types.String `tfsdk:"role"`
	Name   types.String `tfsdk:"name"`
}

func NewOrgUserDataSource() datasource.DataSource {
	return &OrgUserDataSource{}
}

func (d *OrgUserDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_org_user"
}

func (d *OrgUserDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Looks up an existing organization user by email. Membership and role are managed outside Terraform; this data source only reads the current state so you can wire the user into other resources (e.g. team membership). Errors if no user with the given email is a member of the organization.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "OrgUser ID. Use this value when another resource needs an `org_user_id`.",
			},
			"org_id": schema.StringAttribute{
				Required:    true,
				Description: "Organization ID to search within.",
			},
			"email": schema.StringAttribute{
				Required:    true,
				Description: "Email address of the organization user. Matched case-insensitively against the server's normalized (lowercased, trimmed) value.",
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(3),
				},
			},
			"user_id": schema.StringAttribute{
				Computed:    true,
				Description: "Underlying User ID (distinct from the OrgUser ID).",
			},
			"role": schema.StringAttribute{
				Computed:    true,
				Description: "Current organization role: owner, admin, member, viewer, billing_manager, or collaborator.",
			},
			"name": schema.StringAttribute{
				Computed:    true,
				Description: "Display name from the user's profile.",
			},
		},
	}
}

func (d *OrgUserDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = tfutil.FromProviderData[client.Client](req.ProviderData, &resp.Diagnostics)
}

func (d *OrgUserDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg orgUserDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	orgID, err := strconv.ParseUint(cfg.OrgID.ValueString(), 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("invalid org_id", err.Error())
		return
	}

	email := strings.ToLower(strings.TrimSpace(cfg.Email.ValueString()))

	out, err := d.client.API.ListOrgUsers(ctx, &generated.ListOrgUsersRequestOptions{
		PathParams: &generated.ListOrgUsersPath{OrgID: orgID},
		Query:      &generated.ListOrgUsersQuery{Email: &email},
	})
	if err != nil {
		tfutil.AddAPIError(&resp.Diagnostics, "list org users failed", err)
		return
	}

	var matches []*generated.OrgUserDetail
	for i := range out.Users {
		if strings.ToLower(out.Users[i].Email) == email {
			matches = append(matches, &out.Users[i])
		}
	}

	switch len(matches) {
	case 1:
		u := matches[0]
		cfg.ID = types.StringValue(strconv.FormatUint(u.ID, 10))
		cfg.UserID = types.StringValue(strconv.FormatUint(u.UserID, 10))
		cfg.Email = types.StringValue(u.Email)
		cfg.Role = types.StringValue(string(u.Role))
		cfg.Name = types.StringValue(u.Name)
		resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
	case 0:
		resp.Diagnostics.AddError(
			"org user not found",
			fmt.Sprintf("no organization user with email %q in org %s. "+
				"Add the user out-of-band (UI or API) before referencing them from Terraform.",
				email, cfg.OrgID.ValueString()),
		)
	default:
		ids := make([]uint64, len(matches))
		for i, m := range matches {
			ids[i] = m.ID
		}
		resp.Diagnostics.AddError(
			"multiple org users matched email",
			fmt.Sprintf("expected exactly 1 org user with email %q, got %d (ids=%v)",
				email, len(matches), ids),
		)
	}
}
