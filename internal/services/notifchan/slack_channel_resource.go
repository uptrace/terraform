package notifchan

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/uptrace/terraform/internal/client"
	"github.com/uptrace/terraform/internal/generated"
	"github.com/uptrace/terraform/internal/tfutil"
)

var (
	_ resource.Resource                   = &SlackChannelResource{}
	_ resource.ResourceWithConfigure      = &SlackChannelResource{}
	_ resource.ResourceWithImportState    = &SlackChannelResource{}
	_ resource.ResourceWithValidateConfig = &SlackChannelResource{}
)

var slackAuthMethods = []string{"webhook", "token"}

type SlackChannelResource struct {
	client *client.Client
}

func NewSlackChannelResource() resource.Resource {
	return &SlackChannelResource{}
}

type slackChannelModel struct {
	ID         types.String `tfsdk:"id"`
	ProjectID  types.String `tfsdk:"project_id"`
	Name       types.String `tfsdk:"name"`
	Status     types.String `tfsdk:"status"`
	MatchAll   types.Bool   `tfsdk:"match_all"`
	Condition  types.String `tfsdk:"condition"`
	Priorities types.List   `tfsdk:"priorities"`
	MonitorIDs types.Set    `tfsdk:"monitor_ids"`

	AuthMethod types.String `tfsdk:"auth_method"`
	WebhookURL types.String `tfsdk:"webhook_url"`
	Token      types.String `tfsdk:"token"`
	Channel    types.String `tfsdk:"channel"`
}

var slackCRUD = channelCRUD[slackChannelModel]{
	TypeName:  "slack_channel",
	ProjectID: func(m *slackChannelModel) types.String { return m.ProjectID },
	ID:        func(m *slackChannelModel) types.String { return m.ID },
	Name:      func(m *slackChannelModel) types.String { return m.Name },
	Build:     buildSlackRequest,
	Apply:     applySlackToModel,
}

func (r *SlackChannelResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_slack_channel"
}

func (r *SlackChannelResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attrs := sharedAttributes()
	attrs["auth_method"] = schema.StringAttribute{
		Required:    true,
		Description: "Authentication method: webhook or token.",
		Validators: []validator.String{
			stringvalidator.OneOf(slackAuthMethods...),
		},
	}
	attrs["webhook_url"] = schema.StringAttribute{
		Optional:    true,
		Sensitive:   true,
		Description: "Slack webhook URL (required when auth_method is webhook).",
	}
	attrs["token"] = schema.StringAttribute{
		Optional:    true,
		Sensitive:   true,
		Description: "Slack bot token (required when auth_method is token).",
	}
	attrs["channel"] = schema.StringAttribute{
		Optional:    true,
		Description: "Slack channel or user (required when auth_method is token).",
	}
	resp.Schema = schema.Schema{
		Description: "Manages an Uptrace Slack notification channel.",
		Attributes:  attrs,
	}
}

func (r *SlackChannelResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = tfutil.FromProviderData[client.Client](req.ProviderData, &resp.Diagnostics)
}

func (r *SlackChannelResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var cfg slackChannelModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	validateMatchAllRule(cfg.MatchAll, cfg.MonitorIDs, &resp.Diagnostics)

	if cfg.AuthMethod.IsUnknown() || cfg.AuthMethod.IsNull() {
		return
	}
	switch cfg.AuthMethod.ValueString() {
	case "webhook":
		requireField(cfg.WebhookURL, path.Root("webhook_url"), `auth_method is "webhook"`, &resp.Diagnostics)
		forbidField(cfg.Token, path.Root("token"), `auth_method is "webhook"`, &resp.Diagnostics)
		forbidField(cfg.Channel, path.Root("channel"), `auth_method is "webhook"`, &resp.Diagnostics)
	case "token":
		requireField(cfg.Token, path.Root("token"), `auth_method is "token"`, &resp.Diagnostics)
		requireField(cfg.Channel, path.Root("channel"), `auth_method is "token"`, &resp.Diagnostics)
		forbidField(cfg.WebhookURL, path.Root("webhook_url"), `auth_method is "token"`, &resp.Diagnostics)
	}
}

func (r *SlackChannelResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan slackChannelModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(channelCreate(ctx, r.client, &plan, slackCRUD)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *SlackChannelResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state slackChannelModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	remove, diags := channelRead(ctx, r.client, &state, slackCRUD)
	resp.Diagnostics.Append(diags...)
	if remove {
		resp.State.RemoveResource(ctx)
		return
	}
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *SlackChannelResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan slackChannelModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(channelUpdate(ctx, r.client, &plan, slackCRUD)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *SlackChannelResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state slackChannelModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(channelDelete(ctx, r.client, &state, slackCRUD)...)
}

// ImportState accepts "<project_id>:<channel_id>".
func (r *SlackChannelResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	tfutil.ImportStateCompoundID(ctx, req, resp,
		tfutil.ImportField{Name: "project_id", Parse: tfutil.ParseUint32},
		tfutil.ImportField{Name: "channel_id", Parse: tfutil.ParseInt64},
	)
}

func buildSlackRequest(ctx context.Context, m *slackChannelModel) (*generated.NotificationChannelRequest, diag.Diagnostics) {
	var diags diag.Diagnostics

	body := &generated.NotificationChannelRequest{Type: generated.NotificationChannelRequestTypeSlack}
	diags.Append(buildSharedRequest(ctx, sharedFields{
		Name: &m.Name, MatchAll: &m.MatchAll, Condition: &m.Condition,
		Priorities: &m.Priorities, MonitorIDs: &m.MonitorIDs,
	}, body)...)
	if diags.HasError() {
		return nil, diags
	}

	p := generated.SlackParams{}
	if !m.AuthMethod.IsNull() && !m.AuthMethod.IsUnknown() {
		v := generated.SlackParamsAuthMethod(m.AuthMethod.ValueString())
		p.AuthMethod = &v
	}
	p.WebhookURL = valueStringToPtr(m.WebhookURL)
	p.Token = valueStringToPtr(m.Token)
	p.Channel = valueStringToPtr(m.Channel)

	oneOf := &generated.NotificationChannelRequest_Params_OneOf{}
	if err := oneOf.FromSlackParams(p); err != nil {
		diags.AddError("failed to set slack params", err.Error())
		return nil, diags
	}
	body.Params = generated.NotificationChannelRequest_Params{NotificationChannelRequest_Params_OneOf: oneOf}

	return body, diags
}

func applySlackToModel(ctx context.Context, ch *generated.NotificationChannel, dst *slackChannelModel) diag.Diagnostics {
	var diags diag.Diagnostics

	rejectChannelType(generated.Slack, ch, &diags)
	if diags.HasError() {
		return diags
	}

	diags.Append(applyShared(ctx, ch, sharedFields{
		ID: &dst.ID, Name: &dst.Name, Status: &dst.Status,
		MatchAll: &dst.MatchAll, Condition: &dst.Condition,
		Priorities: &dst.Priorities, MonitorIDs: &dst.MonitorIDs,
	})...)
	if diags.HasError() {
		return diags
	}

	if ch.Params.NotificationChannel_Params_OneOf == nil {
		return diags
	}
	p, err := ch.Params.NotificationChannel_Params_OneOf.AsSlackParams()
	if err != nil {
		diags.AddWarning("failed to decode slack params", err.Error())
		return diags
	}
	dst.AuthMethod = tfutil.EnumToValue(p.AuthMethod)
	dst.WebhookURL = tfutil.PreserveIfEmptyPtr(p.WebhookURL, dst.WebhookURL)
	dst.Token = tfutil.PreserveIfEmptyPtr(p.Token, dst.Token)
	dst.Channel = tfutil.PreserveIfEmptyPtr(p.Channel, dst.Channel)

	return diags
}
