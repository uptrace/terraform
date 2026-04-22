package monitor

import (
	"context"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/uptrace/terraform/internal/client"
	"github.com/uptrace/terraform/internal/generated"
	"github.com/uptrace/terraform/internal/tfutil"
)

var (
	_ resource.Resource                   = &MetricMonitorResource{}
	_ resource.ResourceWithConfigure      = &MetricMonitorResource{}
	_ resource.ResourceWithImportState    = &MetricMonitorResource{}
	_ resource.ResourceWithValidateConfig = &MetricMonitorResource{}
)

type MetricMonitorResource struct {
	client *client.Client
}

func NewMetricMonitorResource() resource.Resource {
	return &MetricMonitorResource{}
}

type metricMonitorModel struct {
	ID                    types.String       `tfsdk:"id"`
	ProjectID             types.String       `tfsdk:"project_id"`
	Name                  types.String       `tfsdk:"name"`
	NotifyEveryoneByEmail types.Bool         `tfsdk:"notify_everyone_by_email"`
	TrendAggFunc          types.String       `tfsdk:"trend_agg_func"`
	TrendSensitivity      types.String       `tfsdk:"trend_sensitivity"`
	TeamIDs               types.Set          `tfsdk:"team_ids"`
	ChannelIDs            types.Set          `tfsdk:"channel_ids"`
	Status                types.String       `tfsdk:"status"`
	Params                *metricParamsModel `tfsdk:"params"`
}

func (r *MetricMonitorResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_metric_monitor"
}

func (r *MetricMonitorResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attrs := sharedAttributes()
	attrs["params"] = metricParamsAttribute()
	resp.Schema = schema.Schema{
		Description: "Manages an Uptrace metric monitor. Evaluates an MQL query on a schedule with a manual threshold or automatic trend-based detector.",
		Attributes:  attrs,
	}
}

func (r *MetricMonitorResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = tfutil.FromProviderData[client.Client](req.ProviderData, &resp.Diagnostics)
}

func (r *MetricMonitorResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var cfg metricMonitorModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if cfg.Params == nil {
		return
	}
	validateDetector(cfg.Params, &resp.Diagnostics)
}

func (r *MetricMonitorResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan metricMonitorModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	projectID64, err := strconv.ParseUint(plan.ProjectID.ValueString(), 10, 32)
	if err != nil {
		resp.Diagnostics.AddError("invalid project_id", err.Error())
		return
	}
	projectID := uint32(projectID64)

	body, diags := buildMetricCreateBody(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Info(ctx, "creating metric monitor", map[string]any{
		"project_id": plan.ProjectID.ValueString(),
		"name":       plan.Name.ValueString(),
	})

	out, err := r.client.API.CreateMonitor(ctx, &generated.CreateMonitorRequestOptions{
		PathParams: &generated.CreateMonitorPath{ProjectID: projectID},
		Body:       body,
	})
	if err != nil {
		resp.Diagnostics.AddError("create monitor failed", err.Error())
		return
	}

	resp.Diagnostics.Append(applyMetricMonitorToModel(&out.Monitor, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *MetricMonitorResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state metricMonitorModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	projectID64, err := strconv.ParseUint(state.ProjectID.ValueString(), 10, 32)
	if err != nil {
		resp.Diagnostics.AddError("invalid project_id", err.Error())
		return
	}
	projectID := uint32(projectID64)
	monitorID, err := strconv.ParseInt(state.ID.ValueString(), 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("invalid monitor id", err.Error())
		return
	}

	out, err := r.client.API.GetMonitor(ctx, &generated.GetMonitorRequestOptions{
		PathParams: &generated.GetMonitorPath{
			ProjectID: projectID,
			MonitorID: monitorID,
		},
	})
	if err != nil {
		if client.IsNotFound(err) || client.IsForbidden(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("read monitor failed", err.Error())
		return
	}

	resp.Diagnostics.Append(applyMetricMonitorToModel(&out.Monitor, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *MetricMonitorResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan metricMonitorModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	projectID64, err := strconv.ParseUint(plan.ProjectID.ValueString(), 10, 32)
	if err != nil {
		resp.Diagnostics.AddError("invalid project_id", err.Error())
		return
	}
	projectID := uint32(projectID64)
	monitorID, err := strconv.ParseInt(plan.ID.ValueString(), 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("invalid monitor id", err.Error())
		return
	}

	body, diags := buildMetricUpdateBody(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Info(ctx, "updating metric monitor", map[string]any{"id": plan.ID.ValueString()})

	out, err := r.client.API.UpdateMonitor(ctx, &generated.UpdateMonitorRequestOptions{
		PathParams: &generated.UpdateMonitorPath{
			ProjectID: projectID,
			MonitorID: monitorID,
		},
		Body: body,
	})
	if err != nil {
		resp.Diagnostics.AddError("update monitor failed", err.Error())
		return
	}

	resp.Diagnostics.Append(applyMetricMonitorToModel(&out.Monitor, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *MetricMonitorResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state metricMonitorModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	projectID64, err := strconv.ParseUint(state.ProjectID.ValueString(), 10, 32)
	if err != nil {
		resp.Diagnostics.AddError("invalid project_id", err.Error())
		return
	}
	projectID := uint32(projectID64)
	monitorID, err := strconv.ParseInt(state.ID.ValueString(), 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("invalid monitor id", err.Error())
		return
	}

	tflog.Info(ctx, "deleting metric monitor", map[string]any{"id": state.ID.ValueString()})

	_, err = r.client.API.DeleteMonitor(ctx, &generated.DeleteMonitorRequestOptions{
		PathParams: &generated.DeleteMonitorPath{
			ProjectID: projectID,
			MonitorID: monitorID,
		},
	})
	if err != nil && !client.IsNotFound(err) && !client.IsForbidden(err) {
		resp.Diagnostics.AddError("delete monitor failed", err.Error())
	}
}

// ImportState accepts "<project_id>:<monitor_id>".
func (r *MetricMonitorResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	tfutil.ImportStateCompoundID(ctx, req, resp,
		tfutil.ImportField{Name: "project_id", Parse: tfutil.ParseUint32},
		tfutil.ImportField{Name: "monitor_id", Parse: tfutil.ParseInt64},
	)
}
