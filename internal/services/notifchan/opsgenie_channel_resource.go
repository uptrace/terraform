package notifchan

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/uptrace/terraform/internal/client"
	"github.com/uptrace/terraform/internal/generated"
	"github.com/uptrace/terraform/internal/tfutil"
)

var (
	_ resource.Resource                   = &OpsgenieChannelResource{}
	_ resource.ResourceWithConfigure      = &OpsgenieChannelResource{}
	_ resource.ResourceWithImportState    = &OpsgenieChannelResource{}
	_ resource.ResourceWithValidateConfig = &OpsgenieChannelResource{}
)

var opsgeniePriorities = []string{"P1", "P2", "P3", "P4", "P5"}

type OpsgenieChannelResource struct {
	client *client.Client
}

func NewOpsgenieChannelResource() resource.Resource {
	return &OpsgenieChannelResource{}
}

type opsgenieChannelModel struct {
	ID         types.String `tfsdk:"id"`
	ProjectID  types.String `tfsdk:"project_id"`
	Name       types.String `tfsdk:"name"`
	Status     types.String `tfsdk:"status"`
	MatchAll   types.Bool   `tfsdk:"match_all"`
	Condition  types.String `tfsdk:"condition"`
	Priorities types.List   `tfsdk:"priorities"`
	MonitorIDs types.Set    `tfsdk:"monitor_ids"`

	APIKey   types.String `tfsdk:"api_key"`
	Priority types.String `tfsdk:"priority"`
}

var opsgenieCRUD = channelCRUD[opsgenieChannelModel]{
	TypeName:  "opsgenie_channel",
	ProjectID: func(m *opsgenieChannelModel) types.String { return m.ProjectID },
	ID:        func(m *opsgenieChannelModel) types.String { return m.ID },
	Name:      func(m *opsgenieChannelModel) types.String { return m.Name },
	Build:     buildOpsgenieRequest,
	Apply:     applyOpsgenieToModel,
}

func (r *OpsgenieChannelResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_opsgenie_channel"
}

func (r *OpsgenieChannelResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attrs := sharedAttributes()
	attrs["api_key"] = schema.StringAttribute{
		Required:    true,
		Sensitive:   true,
		Description: "Opsgenie API key.",
	}
	attrs["priority"] = schema.StringAttribute{
		Required:    true,
		Description: "Opsgenie alert priority (P1–P5).",
		Validators: []validator.String{
			stringvalidator.OneOf(opsgeniePriorities...),
		},
	}
	resp.Schema = schema.Schema{
		Description: "Manages an Uptrace Opsgenie notification channel.",
		Attributes:  attrs,
	}
}

func (r *OpsgenieChannelResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = tfutil.FromProviderData[client.Client](req.ProviderData, &resp.Diagnostics)
}

func (r *OpsgenieChannelResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var cfg opsgenieChannelModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	validateMatchAllRule(cfg.MatchAll, cfg.MonitorIDs, &resp.Diagnostics)
}

func (r *OpsgenieChannelResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan opsgenieChannelModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(channelCreate(ctx, r.client, &plan, opsgenieCRUD)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *OpsgenieChannelResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state opsgenieChannelModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	remove, diags := channelRead(ctx, r.client, &state, opsgenieCRUD)
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

func (r *OpsgenieChannelResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan opsgenieChannelModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(channelUpdate(ctx, r.client, &plan, opsgenieCRUD)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *OpsgenieChannelResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state opsgenieChannelModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(channelDelete(ctx, r.client, &state, opsgenieCRUD)...)
}

// ImportState accepts "<project_id>:<channel_id>".
func (r *OpsgenieChannelResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	tfutil.ImportStateCompoundID(ctx, req, resp,
		tfutil.ImportField{Name: "project_id", Parse: tfutil.ParseUint32},
		tfutil.ImportField{Name: "channel_id", Parse: tfutil.ParseInt64},
	)
}

func buildOpsgenieRequest(ctx context.Context, m *opsgenieChannelModel) (*generated.NotificationChannelRequest, diag.Diagnostics) {
	var diags diag.Diagnostics

	body := &generated.NotificationChannelRequest{Type: generated.NotificationChannelRequestTypeOpsgenie}
	diags.Append(buildSharedRequest(ctx, sharedFields{
		Name: &m.Name, MatchAll: &m.MatchAll, Condition: &m.Condition,
		Priorities: &m.Priorities, MonitorIDs: &m.MonitorIDs,
	}, body)...)
	if diags.HasError() {
		return nil, diags
	}

	oneOf := &generated.NotificationChannelRequest_Params_OneOf{}
	if err := oneOf.FromOpsgenieParams(generated.OpsgenieParams{
		APIKey:   m.APIKey.ValueString(),
		Priority: generated.OpsgenieParamsPriority(m.Priority.ValueString()),
	}); err != nil {
		diags.AddError("failed to set opsgenie params", err.Error())
		return nil, diags
	}
	body.Params = generated.NotificationChannelRequest_Params{NotificationChannelRequest_Params_OneOf: oneOf}

	return body, diags
}

func applyOpsgenieToModel(ctx context.Context, ch *generated.NotificationChannel, dst *opsgenieChannelModel) diag.Diagnostics {
	var diags diag.Diagnostics

	rejectChannelType(generated.Opsgenie, ch, &diags)
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
	p, err := ch.Params.NotificationChannel_Params_OneOf.AsOpsgenieParams()
	if err != nil {
		diags.AddWarning("failed to decode opsgenie params", err.Error())
		return diags
	}
	dst.APIKey = tfutil.PreserveIfEmpty(p.APIKey, dst.APIKey)
	dst.Priority = types.StringValue(string(p.Priority))

	return diags
}
