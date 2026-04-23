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
	ID        types.String `tfsdk:"id"`
	OrgID     types.String `tfsdk:"org_id"`
	Email     types.String `tfsdk:"email"`
	Role      types.String `tfsdk:"role"`
	TeamID    types.String `tfsdk:"team_id"`
	State     types.String `tfsdk:"state"`
	OrgUserID types.String `tfsdk:"org_user_id"`
	UserID    types.String `tfsdk:"user_id"`
}

func NewOrgUserResource() resource.Resource {
	return &OrgUserResource{}
}

func (r *OrgUserResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_org_user"
}

func (r *OrgUserResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Sends an organization invitation. The invitation starts in `sent` state; the invitee materializes their OrgUser row by clicking the emailed link. Terraform then discovers the materialized row on the next refresh and populates `org_user_id`/`user_id`. If the invitee is already a member, the backend marks the invitation `accepted` immediately and the resource is populated in the same apply. Role is mutable only after acceptance; `email`, `org_id`, and `team_id` force recreation.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "Invitation ID (32-character hex). Stable across the invite and the resulting membership.",
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
				Description: "Organization role: owner, admin, member, viewer, billing_manager, or collaborator. Mutable only after the invitation is accepted.",
				Validators: []validator.String{
					stringvalidator.OneOf(orgUserRoles...),
				},
			},
			"team_id": schema.StringAttribute{
				Optional:    true,
				Description: "Optional team to add the invitee to on acceptance. Manage ongoing team membership with uptrace_team_user. Changing this forces recreation.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"state": schema.StringAttribute{
				Computed:    true,
				Description: "Invitation lifecycle state: `sent`, `accepted`, `canceled`, or `expired`. `canceled` and `expired` invitations are removed from Terraform state on refresh.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"org_user_id": schema.StringAttribute{
				Computed:    true,
				Description: "ID of the materialized OrgUser row. Null while the invitation is `sent`; populated once the invitee accepts.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"user_id": schema.StringAttribute{
				Computed:    true,
				Description: "Underlying User ID. Null while the invitation is `sent`; populated once the invitee accepts.",
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
		tfutil.AddAPIError(&resp.Diagnostics, "create invite failed", err)
		return
	}

	invite := inviteResp.Invite
	plan.ID = types.StringValue(invite.ID)
	plan.State = types.StringValue(string(invite.State))

	// Already-a-member shortcut: the backend returns state=accepted with no
	// email sent and the existing OrgUser row preserved. If the row isn't
	// visible yet (eventual consistency), leave OrgUserID/UserID null — the
	// next Read will find it and reconcile via the pending branch.
	plan.OrgUserID = types.StringNull()
	plan.UserID = types.StringNull()
	if invite.State == generated.Accepted {
		orgUser, err := r.findOrgUserByEmail(ctx, orgID, email)
		if err != nil {
			tfutil.AddAPIError(&resp.Diagnostics, "locate org user failed", err)
			return
		}
		if orgUser != nil {
			plan.OrgUserID = types.StringValue(strconv.FormatUint(orgUser.ID, 10))
			plan.UserID = types.StringValue(strconv.FormatUint(orgUser.UserID, 10))
			plan.Role = types.StringValue(string(orgUser.Role))
		}
	}

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

	if !state.OrgUserID.IsNull() && state.OrgUserID.ValueString() != "" {
		r.readAsMember(ctx, orgID, &state, resp)
		return
	}

	r.readAsPending(ctx, orgID, &state, resp)
}

// readAsMember refreshes an accepted membership. The OrgUser row is the source
// of truth; we don't re-fetch the invitation (once accepted it's decoupled).
func (r *OrgUserResource) readAsMember(ctx context.Context, orgID uint64, state *orgUserModel, resp *resource.ReadResponse) {
	orgUserID, err := strconv.ParseUint(state.OrgUserID.ValueString(), 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("invalid org_user_id", err.Error())
		return
	}

	out, err := r.client.API.GetOrgUser(ctx, &generated.GetOrgUserRequestOptions{
		PathParams: &generated.GetOrgUserPath{OrgID: orgID, OrgUserID: orgUserID},
	})
	if err != nil {
		if tfutil.IsDeleteGone(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		tfutil.AddAPIError(&resp.Diagnostics, "read org user failed", err)
		return
	}

	state.Role = types.StringValue(string(out.OrgUser.Role))
	state.UserID = types.StringValue(strconv.FormatUint(out.OrgUser.UserID, 10))
	state.Email = types.StringValue(out.User.Email)
	state.State = types.StringValue(string(generated.Accepted))

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

// readAsPending refreshes an invitation that has not yet materialized an
// OrgUser. It transitions the resource to "accepted" if the invitee joined
// between applies, or removes it if the invitation was canceled or expired.
func (r *OrgUserResource) readAsPending(ctx context.Context, orgID uint64, state *orgUserModel, resp *resource.ReadResponse) {
	invite, err := r.findInviteByID(ctx, orgID, state.ID.ValueString())
	if err != nil {
		tfutil.AddAPIError(&resp.Diagnostics, "read invitation failed", err)
		return
	}
	if invite == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	switch invite.State {
	case generated.Canceled, generated.Expired:
		resp.State.RemoveResource(ctx)
		return
	case generated.Sent:
		state.State = types.StringValue(string(invite.State))
		state.Email = types.StringValue(invite.Email)
		state.Role = types.StringValue(string(invite.Role))
		resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
		return
	case generated.Accepted:
		orgUser, err := r.findOrgUserByEmail(ctx, orgID, invite.Email)
		if err != nil {
			tfutil.AddAPIError(&resp.Diagnostics, "locate org user after acceptance failed", err)
			return
		}
		state.State = types.StringValue(string(generated.Accepted))
		state.Email = types.StringValue(invite.Email)
		// If the OrgUser row isn't visible yet, leave ids null so the next
		// refresh retries rather than failing the apply.
		if orgUser != nil {
			state.OrgUserID = types.StringValue(strconv.FormatUint(orgUser.ID, 10))
			state.UserID = types.StringValue(strconv.FormatUint(orgUser.UserID, 10))
			state.Role = types.StringValue(string(orgUser.Role))
		} else {
			state.Role = types.StringValue(string(invite.Role))
		}
		resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
		return
	default:
		resp.Diagnostics.AddError("unknown invitation state", fmt.Sprintf("got %q", invite.State))
	}
}

func (r *OrgUserResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state orgUserModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if state.OrgUserID.IsNull() || state.OrgUserID.ValueString() == "" {
		resp.Diagnostics.AddError(
			"cannot update role while invitation is pending",
			"The invitee has not yet accepted the invitation, so there is no OrgUser row to update. "+
				"Wait for acceptance, or taint the resource to cancel the invite and re-invite with the new role.",
		)
		return
	}

	orgID, err := strconv.ParseUint(plan.OrgID.ValueString(), 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("invalid org_id", err.Error())
		return
	}
	orgUserID, err := strconv.ParseUint(state.OrgUserID.ValueString(), 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("invalid org_user_id", err.Error())
		return
	}

	tflog.Info(ctx, "updating org user role", map[string]any{"org_user_id": state.OrgUserID.ValueString()})

	if _, err := r.client.API.UpdateOrgUserRole(ctx, &generated.UpdateOrgUserRoleRequestOptions{
		PathParams: &generated.UpdateOrgUserRolePath{OrgID: orgID, OrgUserID: orgUserID},
		Body: &generated.OrgUserRoleUpdateRequest{
			Role: generated.UserRole(plan.Role.ValueString()),
		},
	}); err != nil {
		tfutil.AddAPIError(&resp.Diagnostics, "update org user role failed", err)
		return
	}

	// UpdateOrgUserRole returns the pre-update row (no RETURNING clause on the
	// SQL UPDATE), so GetOrgUser is the only way to observe the post-update
	// role. Without this, a backend that accepts the call but silently refuses
	// the change would leave state out of sync until the next plan.
	out, err := r.client.API.GetOrgUser(ctx, &generated.GetOrgUserRequestOptions{
		PathParams: &generated.GetOrgUserPath{OrgID: orgID, OrgUserID: orgUserID},
	})
	if err != nil {
		tfutil.AddAPIError(&resp.Diagnostics, "confirm org user role failed", err)
		return
	}

	plan.ID = state.ID
	plan.State = state.State
	plan.OrgUserID = state.OrgUserID
	plan.UserID = types.StringValue(strconv.FormatUint(out.OrgUser.UserID, 10))
	plan.Role = types.StringValue(string(out.OrgUser.Role))

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

	// Resolve the effective OrgUserID at destroy-time. If state shows the
	// invitation as still pending, probe by email to catch a plan→apply
	// race where the invitee accepted since the last refresh — the backend's
	// CancelOrgInvite force-transitions any state to canceled and would
	// orphan the just-materialized OrgUser row if we took that path blindly.
	orgUserIDStr := state.OrgUserID.ValueString()
	if (state.OrgUserID.IsNull() || orgUserIDStr == "") && state.Email.ValueString() != "" {
		orgUser, err := r.findOrgUserByEmail(ctx, orgID, state.Email.ValueString())
		if err != nil {
			tfutil.AddAPIError(&resp.Diagnostics, "probe org user before destroy failed", err)
			return
		}
		if orgUser != nil {
			orgUserIDStr = strconv.FormatUint(orgUser.ID, 10)
		}
	}

	if orgUserIDStr != "" {
		orgUserID, err := strconv.ParseUint(orgUserIDStr, 10, 64)
		if err != nil {
			resp.Diagnostics.AddError("invalid org_user_id", err.Error())
			return
		}
		tflog.Info(ctx, "removing org user", map[string]any{"org_user_id": orgUserIDStr})
		// RemoveOrgUser cascade-cancels any lingering invitations for the
		// same email (see app/core/org_handler.go:425-451).
		_, err = r.client.API.RemoveOrgUser(ctx, &generated.RemoveOrgUserRequestOptions{
			PathParams: &generated.RemoveOrgUserPath{OrgID: orgID, OrgUserID: orgUserID},
		})
		if err != nil && !tfutil.IsDeleteGone(err) {
			tfutil.AddAPIError(&resp.Diagnostics, "remove org user failed", err)
		}
		return
	}

	tflog.Info(ctx, "canceling invitation", map[string]any{"invite_id": state.ID.ValueString()})
	_, err = r.client.API.CancelOrgInvite(ctx, &generated.CancelOrgInviteRequestOptions{
		PathParams: &generated.CancelOrgInvitePath{OrgID: orgID, InviteID: state.ID.ValueString()},
	})
	if err != nil && !tfutil.IsDeleteGone(err) {
		tfutil.AddAPIError(&resp.Diagnostics, "cancel invitation failed", err)
	}
}

// ImportState accepts "<org_id>:<invite_id>". All other attributes are
// populated from the API on the subsequent Read.
func (r *OrgUserResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	tfutil.ImportStateCompoundID(ctx, req, resp,
		tfutil.ImportField{Name: "org_id", Parse: tfutil.ParseUint64},
		tfutil.ImportField{Name: "invite_id"},
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

// findOrgUserByEmail locates an OrgUser by email via case-insensitive
// substring match then client-side exact filter. Returns (nil, nil) when no
// match exists so callers can treat it as "not yet visible" (eventual
// consistency) rather than a hard error. Multi-match is a unique-email
// invariant violation and returns an error.
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
		return nil, nil
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

// findInviteByID looks up an invitation by its hex ID. The backend has no
// GetOrgInvite endpoint, so we list all invitations for the org and filter
// client-side. Returns nil (not an error) when the invitation is absent.
func (r *OrgUserResource) findInviteByID(ctx context.Context, orgID uint64, inviteID string) (*generated.UserInvite, error) {
	out, err := r.client.API.ListOrgInvites(ctx, &generated.ListOrgInvitesRequestOptions{
		PathParams: &generated.ListOrgInvitesPath{OrgID: orgID},
	})
	if err != nil {
		if tfutil.IsDeleteGone(err) {
			return nil, nil
		}
		return nil, err
	}
	for i := range out.Invites {
		if out.Invites[i].ID == inviteID {
			return &out.Invites[i], nil
		}
	}
	return nil, nil
}
