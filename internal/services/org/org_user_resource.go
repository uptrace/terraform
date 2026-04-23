package org

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/uptrace/terraform/internal/client"
	"github.com/uptrace/terraform/internal/generated"
	"github.com/uptrace/terraform/internal/tfutil"
)

var (
	_ resource.Resource                = &OrgUserResource{}
	_ resource.ResourceWithConfigure   = &OrgUserResource{}
	_ resource.ResourceWithImportState = &OrgUserResource{}
)

var orgUserRoles = []string{"owner", "admin", "member", "viewer", "billing_manager", "collaborator"}

type OrgUserResource struct {
	client *client.Client
}

type orgUserModel struct {
	ID       types.String `tfsdk:"id"`
	OrgID    types.String `tfsdk:"org_id"`
	Email    types.String `tfsdk:"email"`
	Role     types.String `tfsdk:"role"`
	TeamID   types.String `tfsdk:"team_id"`
	UserID   types.String `tfsdk:"user_id"`
	InviteID types.String `tfsdk:"invite_id"`
}

func NewOrgUserResource() resource.Resource {
	return &OrgUserResource{}
}

func (r *OrgUserResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_org_user"
}

func (r *OrgUserResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages an organization user. On create, sends an invitation to the given email and auto-accepts it so the OrgUser row is materialized in a single apply. Role is mutable; email, org_id, and team_id force recreation.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "OrgUser ID.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"org_id": schema.StringAttribute{
				Required:    true,
				Description: "Organization ID. Changing this forces recreation.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"email": schema.StringAttribute{
				Required: true,
				Description: "Email address used to invite the user. Must be lowercase " +
					"and trimmed; the server normalizes emails on insert, so storing " +
					"a mixed-case value in config would cause a perpetual diff. " +
					"Changing this forces recreation.",
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(3),
					lowercaseEmailValidator{},
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"role": schema.StringAttribute{
				Required:    true,
				Description: "Organization role: owner, admin, member, viewer, billing_manager, or collaborator.",
				Validators: []validator.String{
					stringvalidator.OneOf(orgUserRoles...),
				},
			},
			"team_id": schema.StringAttribute{
				Optional:    true,
				Description: "Optional team to add the invitee to on acceptance. Only used during create; manage ongoing team membership with uptrace_team_user. Changing this forces recreation.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"user_id": schema.StringAttribute{
				Computed:    true,
				Description: "Underlying User ID (distinct from OrgUser ID).",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"invite_id": schema.StringAttribute{
				Computed:    true,
				Description: "ID of the invitation used to materialize this OrgUser. Diagnostic only.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *OrgUserResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = tfutil.FromProviderData[client.Client](req.ProviderData, &resp.Diagnostics)
}

func (r *OrgUserResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan orgUserModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	orgID, err := strconv.ParseUint(plan.OrgID.ValueString(), 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("invalid org_id", err.Error())
		return
	}

	email := strings.ToLower(strings.TrimSpace(plan.Email.ValueString()))

	inviteBody := &generated.UserInviteCreateRequest{
		Email: email,
		Role:  generated.UserRole(plan.Role.ValueString()),
	}
	if !plan.TeamID.IsNull() && !plan.TeamID.IsUnknown() {
		teamID, err := strconv.ParseUint(plan.TeamID.ValueString(), 10, 64)
		if err != nil {
			resp.Diagnostics.AddError("invalid team_id", err.Error())
			return
		}
		inviteBody.TeamID = &teamID
	}

	tflog.Info(ctx, "inviting org user", map[string]any{
		"org_id": plan.OrgID.ValueString(),
		"email":  email,
	})

	inviteResp, err := r.client.API.CreateOrgInvite(ctx, &generated.CreateOrgInviteRequestOptions{
		PathParams: &generated.CreateOrgInvitePath{OrgID: orgID},
		Body:       inviteBody,
	})
	if err != nil {
		resp.Diagnostics.AddError("create invite failed", err.Error())
		return
	}

	inviteID := inviteResp.Invite.ID
	plan.InviteID = types.StringValue(inviteID)

	// If the invitee was already a member of this org, the server returns the
	// invite with state=accepted and does not create a new OrgUser. We reuse
	// the existing membership. Otherwise (state=sent) we immediately accept
	// the invite to materialize the OrgUser.
	if inviteResp.Invite.State != generated.Accepted {
		tflog.Info(ctx, "accepting invite", map[string]any{"invite_id": inviteID})
		if _, err := r.client.API.JoinOrg(ctx, &generated.JoinOrgRequestOptions{
			PathParams: &generated.JoinOrgPath{InviteID: inviteID},
		}); err != nil {
			resp.Diagnostics.AddError("join org failed", err.Error())
			return
		}
	}

	orgUser, err := r.findOrgUserByEmail(ctx, orgID, email)
	if err != nil {
		resp.Diagnostics.AddError("locate org user failed", err.Error())
		return
	}

	plan.ID = types.StringValue(strconv.FormatUint(orgUser.ID, 10))
	plan.UserID = types.StringValue(strconv.FormatUint(orgUser.UserID, 10))
	plan.Role = types.StringValue(string(orgUser.Role))

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *OrgUserResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state orgUserModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	orgID, err := strconv.ParseUint(state.OrgID.ValueString(), 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("invalid org_id", err.Error())
		return
	}
	orgUserID, err := strconv.ParseUint(state.ID.ValueString(), 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("invalid org_user_id", err.Error())
		return
	}

	out, err := r.client.API.GetOrgUser(ctx, &generated.GetOrgUserRequestOptions{
		PathParams: &generated.GetOrgUserPath{OrgID: orgID, OrgUserID: orgUserID},
	})
	if err != nil {
		if client.IsNotFound(err) || client.IsForbidden(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("read org user failed", err.Error())
		return
	}

	state.Role = types.StringValue(string(out.OrgUser.Role))
	state.UserID = types.StringValue(strconv.FormatUint(out.OrgUser.UserID, 10))
	state.Email = types.StringValue(out.User.Email)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *OrgUserResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan orgUserModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	orgID, err := strconv.ParseUint(plan.OrgID.ValueString(), 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("invalid org_id", err.Error())
		return
	}
	orgUserID, err := strconv.ParseUint(plan.ID.ValueString(), 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("invalid org_user_id", err.Error())
		return
	}

	tflog.Info(ctx, "updating org user role", map[string]any{"id": plan.ID.ValueString()})

	// The backend's UpdateUserRole handler currently returns the pre-update
	// OrgUser (no RETURNING clause on its SQL UPDATE), so we cannot trust the
	// response's role. If the API call succeeds, the planned role is what was
	// written; refresh by a subsequent Read if you need a server confirmation.
	if _, err := r.client.API.UpdateOrgUserRole(ctx, &generated.UpdateOrgUserRoleRequestOptions{
		PathParams: &generated.UpdateOrgUserRolePath{OrgID: orgID, OrgUserID: orgUserID},
		Body: &generated.OrgUserRoleUpdateRequest{
			Role: generated.UserRole(plan.Role.ValueString()),
		},
	}); err != nil {
		resp.Diagnostics.AddError("update org user role failed", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *OrgUserResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state orgUserModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	orgID, err := strconv.ParseUint(state.OrgID.ValueString(), 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("invalid org_id", err.Error())
		return
	}
	orgUserID, err := strconv.ParseUint(state.ID.ValueString(), 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("invalid org_user_id", err.Error())
		return
	}

	tflog.Info(ctx, "removing org user", map[string]any{"id": state.ID.ValueString()})

	_, err = r.client.API.RemoveOrgUser(ctx, &generated.RemoveOrgUserRequestOptions{
		PathParams: &generated.RemoveOrgUserPath{OrgID: orgID, OrgUserID: orgUserID},
	})
	if err != nil && !client.IsNotFound(err) && !client.IsForbidden(err) {
		resp.Diagnostics.AddError("remove org user failed", err.Error())
	}
}

// ImportState accepts "<org_id>:<org_user_id>". Email and role are populated
// from the API on the subsequent Read.
func (r *OrgUserResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	tfutil.ImportStateCompoundID(ctx, req, resp,
		tfutil.ImportField{Name: "org_id", Parse: tfutil.ParseUint64},
		tfutil.ImportField{Name: "org_user_id", Parse: tfutil.ParseUint64},
	)
}

// lowercaseEmailValidator rejects non-normalized email input at plan time to
// avoid a perpetual diff against server-normalized state (email is RequiresReplace).
type lowercaseEmailValidator struct{}

func (lowercaseEmailValidator) Description(context.Context) string {
	return "email must be lowercase and trimmed"
}

func (v lowercaseEmailValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (lowercaseEmailValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	raw := req.ConfigValue.ValueString()
	normalized := strings.ToLower(strings.TrimSpace(raw))
	if raw != normalized {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"email must be lowercase and trimmed",
			fmt.Sprintf("got %q; write %q instead. "+
				"The backend lowercases emails, so mixed-case input would cause "+
				"a perpetual diff against state.", raw, normalized),
		)
	}
}

// findOrgUserByEmail locates a freshly-created OrgUser after the invite+join
// chain. The list endpoint matches email as a case-insensitive substring, so
// we filter exact equality client-side and assert exactly one match.
func (r *OrgUserResource) findOrgUserByEmail(ctx context.Context, orgID uint64, email string) (*generated.OrgUserDetail, error) {
	out, err := r.client.API.ListOrgUsers(ctx, &generated.ListOrgUsersRequestOptions{
		PathParams: &generated.ListOrgUsersPath{OrgID: orgID},
		Query:      &generated.ListOrgUsersQuery{Email: &email},
	})
	if err != nil {
		return nil, err
	}

	lower := strings.ToLower(email)
	var matches []*generated.OrgUserDetail
	for i := range out.Users {
		if strings.ToLower(out.Users[i].Email) == lower {
			matches = append(matches, &out.Users[i])
		}
	}

	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return nil, fmt.Errorf("no org user with email %q found after invite "+
			"(ListOrgUsers returned %d rows matching as substring)",
			email, len(out.Users))
	default:
		ids := make([]uint64, len(matches))
		for i, m := range matches {
			ids[i] = m.ID
		}
		return nil, fmt.Errorf("expected exactly 1 org user with email %q, "+
			"got %d (ids=%v) — DB unique constraint may have been violated",
			email, len(matches), ids)
	}
}
