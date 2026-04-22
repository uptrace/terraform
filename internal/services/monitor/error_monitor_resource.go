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
	_ resource.Resource                = &ErrorMonitorResource{}
	_ resource.ResourceWithConfigure   = &ErrorMonitorResource{}
	_ resource.ResourceWithImportState = &ErrorMonitorResource{}
)

type ErrorMonitorResource struct {
	client *client.Client
}

func NewErrorMonitorResource() resource.Resource {
	return &ErrorMonitorResource{}
}

type errorMonitorModel struct {
	ID                    types.String      `tfsdk:"id"`
	ProjectID             types.String      `tfsdk:"project_id"`
	Name                  types.String      `tfsdk:"name"`
	NotifyEveryoneByEmail types.Bool        `tfsdk:"notify_everyone_by_email"`
	TrendAggFunc          types.String      `tfsdk:"trend_agg_func"`
	TrendSensitivity      types.String      `tfsdk:"trend_sensitivity"`
	TeamIDs               types.Set         `tfsdk:"team_ids"`
	ChannelIDs            types.Set         `tfsdk:"channel_ids"`
	Status                types.String      `tfsdk:"status"`
	Params                *errorParamsModel `tfsdk:"params"`
}

func (r *ErrorMonitorResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_error_monitor"
}

func (r *ErrorMonitorResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attrs := sharedAttributes()
	attrs["params"] = errorParamsAttribute()
	resp.Schema = schema.Schema{
		Description: "Manages an Uptrace error monitor. Watches trend anomalies in an MQL query and fires alerts through attached notification channels and/or team email.",
		Attributes:  attrs,
	}
}

func (r *ErrorMonitorResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = tfutil.FromProviderData[client.Client](req.ProviderData, &resp.Diagnostics)
}

func (r *ErrorMonitorResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan errorMonitorModel
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

	body, diags := buildErrorCreateBody(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Info(ctx, "creating error monitor", map[string]any{
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

	resp.Diagnostics.Append(applyErrorMonitorToModel(&out.Monitor, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ErrorMonitorResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state errorMonitorModel
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

	resp.Diagnostics.Append(applyErrorMonitorToModel(&out.Monitor, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *ErrorMonitorResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan errorMonitorModel
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

	body, diags := buildErrorUpdateBody(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Info(ctx, "updating error monitor", map[string]any{"id": plan.ID.ValueString()})

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

	resp.Diagnostics.Append(applyErrorMonitorToModel(&out.Monitor, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ErrorMonitorResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state errorMonitorModel
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

	tflog.Info(ctx, "deleting error monitor", map[string]any{"id": state.ID.ValueString()})

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
func (r *ErrorMonitorResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	tfutil.ImportStateCompoundID(ctx, req, resp,
		tfutil.ImportField{Name: "project_id", Parse: tfutil.ParseUint32},
		tfutil.ImportField{Name: "monitor_id", Parse: tfutil.ParseInt64},
	)
}
