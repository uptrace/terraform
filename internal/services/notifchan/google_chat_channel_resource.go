package notifchan

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/uptrace/terraform/internal/client"
	"github.com/uptrace/terraform/internal/generated"
	"github.com/uptrace/terraform/internal/tfutil"
)

var (
	_ resource.Resource                   = &GoogleChatChannelResource{}
	_ resource.ResourceWithConfigure      = &GoogleChatChannelResource{}
	_ resource.ResourceWithImportState    = &GoogleChatChannelResource{}
	_ resource.ResourceWithValidateConfig = &GoogleChatChannelResource{}
)

type GoogleChatChannelResource struct {
	client *client.Client
}

func NewGoogleChatChannelResource() resource.Resource {
	return &GoogleChatChannelResource{}
}

type googleChatChannelModel struct {
	ID         types.String `tfsdk:"id"`
	ProjectID  types.String `tfsdk:"project_id"`
	Name       types.String `tfsdk:"name"`
	Status     types.String `tfsdk:"status"`
	MatchAll   types.Bool   `tfsdk:"match_all"`
	Condition  types.String `tfsdk:"condition"`
	Priorities types.List   `tfsdk:"priorities"`
	MonitorIDs types.Set    `tfsdk:"monitor_ids"`

	WebhookURL types.String `tfsdk:"webhook_url"`
}

var googleChatCRUD = channelCRUD[googleChatChannelModel]{
	TypeName:  "google_chat_channel",
	ProjectID: func(m *googleChatChannelModel) types.String { return m.ProjectID },
	ID:        func(m *googleChatChannelModel) types.String { return m.ID },
	Name:      func(m *googleChatChannelModel) types.String { return m.Name },
	Build:     buildGoogleChatRequest,
	Apply:     applyGoogleChatToModel,
}

func (r *GoogleChatChannelResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_google_chat_channel"
}

func (r *GoogleChatChannelResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attrs := sharedAttributes()
	attrs["webhook_url"] = schema.StringAttribute{
		Required:    true,
		Sensitive:   true,
		Description: "Google Chat webhook URL.",
	}
	resp.Schema = schema.Schema{
		Description: "Manages an Uptrace Google Chat notification channel.",
		Attributes:  attrs,
	}
}

func (r *GoogleChatChannelResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = tfutil.FromProviderData[client.Client](req.ProviderData, &resp.Diagnostics)
}

func (r *GoogleChatChannelResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var cfg googleChatChannelModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	validateMatchAllRule(cfg.MatchAll, cfg.MonitorIDs, &resp.Diagnostics)
}

func (r *GoogleChatChannelResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan googleChatChannelModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(channelCreate(ctx, r.client, &plan, googleChatCRUD)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *GoogleChatChannelResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state googleChatChannelModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	remove, diags := channelRead(ctx, r.client, &state, googleChatCRUD)
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

func (r *GoogleChatChannelResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan googleChatChannelModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(channelUpdate(ctx, r.client, &plan, googleChatCRUD)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *GoogleChatChannelResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state googleChatChannelModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(channelDelete(ctx, r.client, &state, googleChatCRUD)...)
}

// ImportState accepts "<project_id>:<channel_id>".
func (r *GoogleChatChannelResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	tfutil.ImportStateCompoundID(ctx, req, resp,
		tfutil.ImportField{Name: "project_id", Parse: tfutil.ParseUint32},
		tfutil.ImportField{Name: "channel_id", Parse: tfutil.ParseInt64},
	)
}

func buildGoogleChatRequest(ctx context.Context, m *googleChatChannelModel) (*generated.NotificationChannelRequest, diag.Diagnostics) {
	var diags diag.Diagnostics

	body := &generated.NotificationChannelRequest{Type: generated.NotificationChannelRequestTypeGoogleChat}
	diags.Append(buildSharedRequest(ctx, sharedFields{
		Name: &m.Name, MatchAll: &m.MatchAll, Condition: &m.Condition,
		Priorities: &m.Priorities, MonitorIDs: &m.MonitorIDs,
	}, body)...)
	if diags.HasError() {
		return nil, diags
	}

	oneOf := &generated.NotificationChannelRequest_Params_OneOf{}
	if err := oneOf.FromGoogleChatParams(generated.GoogleChatParams{
		WebhookURL: m.WebhookURL.ValueString(),
	}); err != nil {
		diags.AddError("failed to set google_chat params", err.Error())
		return nil, diags
	}
	body.Params = generated.NotificationChannelRequest_Params{NotificationChannelRequest_Params_OneOf: oneOf}

	return body, diags
}

func applyGoogleChatToModel(ctx context.Context, ch *generated.NotificationChannel, dst *googleChatChannelModel) diag.Diagnostics {
	var diags diag.Diagnostics

	rejectChannelType(generated.GoogleChat, ch, &diags)
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
		dst.WebhookURL = tfutil.PreferPrior(dst.WebhookURL, types.StringNull())
		return diags
	}
	p, err := ch.Params.NotificationChannel_Params_OneOf.AsGoogleChatParams()
	if err != nil {
		diags.AddWarning("failed to decode google_chat params", err.Error())
		return diags
	}
	dst.WebhookURL = tfutil.PreserveIfEmpty(p.WebhookURL, dst.WebhookURL)

	return diags
}
