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
	_ resource.Resource                   = &TelegramChannelResource{}
	_ resource.ResourceWithConfigure      = &TelegramChannelResource{}
	_ resource.ResourceWithImportState    = &TelegramChannelResource{}
	_ resource.ResourceWithValidateConfig = &TelegramChannelResource{}
)

type TelegramChannelResource struct {
	client *client.Client
}

func NewTelegramChannelResource() resource.Resource {
	return &TelegramChannelResource{}
}

type telegramChannelModel struct {
	ID         types.String `tfsdk:"id"`
	ProjectID  types.String `tfsdk:"project_id"`
	Name       types.String `tfsdk:"name"`
	Status     types.String `tfsdk:"status"`
	MatchAll   types.Bool   `tfsdk:"match_all"`
	Condition  types.String `tfsdk:"condition"`
	Priorities types.List   `tfsdk:"priorities"`
	MonitorIDs types.Set    `tfsdk:"monitor_ids"`

	ChatID types.Int64 `tfsdk:"chat_id"`
}

var telegramCRUD = channelCRUD[telegramChannelModel]{
	TypeName:  "telegram_channel",
	ProjectID: func(m *telegramChannelModel) types.String { return m.ProjectID },
	ID:        func(m *telegramChannelModel) types.String { return m.ID },
	Name:      func(m *telegramChannelModel) types.String { return m.Name },
	Build:     buildTelegramRequest,
	Apply:     applyTelegramToModel,
}

func (r *TelegramChannelResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_telegram_channel"
}

func (r *TelegramChannelResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attrs := sharedAttributes()
	attrs["chat_id"] = schema.Int64Attribute{
		Required:    true,
		Description: "Telegram chat ID.",
	}
	resp.Schema = schema.Schema{
		Description: "Manages an Uptrace Telegram notification channel.",
		Attributes:  attrs,
	}
}

func (r *TelegramChannelResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = tfutil.FromProviderData[client.Client](req.ProviderData, &resp.Diagnostics)
}

func (r *TelegramChannelResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var cfg telegramChannelModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	validateMatchAllRule(cfg.MatchAll, cfg.MonitorIDs, &resp.Diagnostics)
}

func (r *TelegramChannelResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan telegramChannelModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(channelCreate(ctx, r.client, &plan, telegramCRUD)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *TelegramChannelResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state telegramChannelModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	remove, diags := channelRead(ctx, r.client, &state, telegramCRUD)
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

func (r *TelegramChannelResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan telegramChannelModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(channelUpdate(ctx, r.client, &plan, telegramCRUD)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *TelegramChannelResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state telegramChannelModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(channelDelete(ctx, r.client, &state, telegramCRUD)...)
}

// ImportState accepts "<project_id>:<channel_id>".
func (r *TelegramChannelResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	tfutil.ImportStateCompoundID(ctx, req, resp,
		tfutil.ImportField{Name: "project_id", Parse: tfutil.ParseUint32},
		tfutil.ImportField{Name: "channel_id", Parse: tfutil.ParseInt64},
	)
}

func buildTelegramRequest(ctx context.Context, m *telegramChannelModel) (*generated.NotificationChannelRequest, diag.Diagnostics) {
	var diags diag.Diagnostics

	body := &generated.NotificationChannelRequest{Type: generated.NotificationChannelRequestTypeTelegram}
	diags.Append(buildSharedRequest(ctx, sharedFields{
		Name: &m.Name, MatchAll: &m.MatchAll, Condition: &m.Condition,
		Priorities: &m.Priorities, MonitorIDs: &m.MonitorIDs,
	}, body)...)
	if diags.HasError() {
		return nil, diags
	}

	oneOf := &generated.NotificationChannelRequest_Params_OneOf{}
	if err := oneOf.FromTelegramParams(generated.TelegramParams{
		ChatID: m.ChatID.ValueInt64(),
	}); err != nil {
		diags.AddError("failed to set telegram params", err.Error())
		return nil, diags
	}
	body.Params = generated.NotificationChannelRequest_Params{NotificationChannelRequest_Params_OneOf: oneOf}

	return body, diags
}

func applyTelegramToModel(ctx context.Context, ch *generated.NotificationChannel, dst *telegramChannelModel) diag.Diagnostics {
	var diags diag.Diagnostics

	rejectChannelType(generated.Telegram, ch, &diags)
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
	p, err := ch.Params.NotificationChannel_Params_OneOf.AsTelegramParams()
	if err != nil {
		diags.AddWarning("failed to decode telegram params", err.Error())
		return diags
	}
	dst.ChatID = types.Int64Value(p.ChatID)

	return diags
}
