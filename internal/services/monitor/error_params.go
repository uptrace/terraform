package monitor

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/uptrace/oapi-codegen-dd/v3/pkg/runtime"

	"github.com/uptrace/terraform/internal/generated"
	"github.com/uptrace/terraform/internal/tfutil"
)

type errorParamsModel struct {
	Metrics []monitorMetricModel `tfsdk:"metrics"`
	Query   types.String         `tfsdk:"query"`
}

func errorParamsAttribute() schema.Attribute {
	return schema.SingleNestedAttribute{
		Required:    true,
		Description: "Error monitor query parameters.",
		Attributes: map[string]schema.Attribute{
			"metrics": monitorMetricsAttribute(),
			"query": schema.StringAttribute{
				Required:    true,
				Description: "MQL query expression that defines which events the monitor watches.",
			},
		},
	}
}

func buildErrorCreateBody(ctx context.Context, m *errorMonitorModel) (*generated.CreateMonitorBody, diag.Diagnostics) {
	req, diags := buildErrorMonitorRequest(ctx, m)
	if diags.HasError() {
		return nil, diags
	}
	return &generated.CreateMonitorBody{
		CreateMonitorBody_OneOf: &generated.CreateMonitorBody_OneOf{
			Either: runtime.NewEitherFromB[generated.MetricMonitorRequest, generated.ErrorMonitorRequest](*req),
		},
	}, diags
}

func buildErrorUpdateBody(ctx context.Context, m *errorMonitorModel) (*generated.UpdateMonitorBody, diag.Diagnostics) {
	req, diags := buildErrorMonitorRequest(ctx, m)
	if diags.HasError() {
		return nil, diags
	}
	return &generated.UpdateMonitorBody{
		UpdateMonitorBody_OneOf: &generated.UpdateMonitorBody_OneOf{
			Either: runtime.NewEitherFromB[generated.MetricMonitorRequest, generated.ErrorMonitorRequest](*req),
		},
	}, diags
}

func buildErrorMonitorRequest(ctx context.Context, m *errorMonitorModel) (*generated.ErrorMonitorRequest, diag.Diagnostics) {
	var diags diag.Diagnostics

	req := &generated.ErrorMonitorRequest{
		Name: m.Name.ValueString(),
		Type: generated.ErrorMonitorRequestTypeError,
	}

	if !m.NotifyEveryoneByEmail.IsNull() && !m.NotifyEveryoneByEmail.IsUnknown() {
		v := m.NotifyEveryoneByEmail.ValueBool()
		req.NotifyEveryoneByEmail = &v
	}
	if !m.TrendAggFunc.IsNull() && !m.TrendAggFunc.IsUnknown() {
		v := generated.ErrorMonitorRequestTrendAggFunc(m.TrendAggFunc.ValueString())
		req.TrendAggFunc = &v
	}
	if !m.TrendSensitivity.IsNull() && !m.TrendSensitivity.IsUnknown() {
		v := generated.ErrorMonitorRequestTrendSensitivity(m.TrendSensitivity.ValueString())
		req.TrendSensitivity = &v
	}

	teamIDs, d := tfutil.SliceFromIntSet(ctx, path.Root("team_ids"), m.TeamIDs)
	diags.Append(d...)
	if d.HasError() {
		return nil, diags
	}
	req.TeamIds = teamIDs

	channelIDs, d := tfutil.SliceFromIntSet(ctx, path.Root("channel_ids"), m.ChannelIDs)
	diags.Append(d...)
	if d.HasError() {
		return nil, diags
	}
	req.ChannelIds = channelIDs

	if m.Params == nil {
		diags.AddAttributeError(
			path.Root("params"),
			"params is required",
			"params block must be set.",
		)
		return nil, diags
	}
	metrics := make([]generated.MonitorMetric, len(m.Params.Metrics))
	for i, mm := range m.Params.Metrics {
		mem := generated.MonitorMetric{Name: mm.Name.ValueString()}
		if !mm.Alias.IsNull() && !mm.Alias.IsUnknown() {
			v := mm.Alias.ValueString()
			mem.Alias = &v
		}
		metrics[i] = mem
	}
	req.Params = generated.ErrorMonitorParams{
		Query:   m.Params.Query.ValueString(),
		Metrics: metrics,
	}

	return req, diags
}

func applyErrorMonitorToModel(mon *generated.Monitor, dst *errorMonitorModel) diag.Diagnostics {
	var diags diag.Diagnostics

	if mon.Type != generated.MonitorTypeError {
		diags.AddError(
			"monitor type mismatch",
			fmt.Sprintf("expected error monitor, API returned %q for monitor id=%d", mon.Type, mon.ID),
		)
		return diags
	}

	applyShared(mon, sharedFields{
		ID:                    &dst.ID,
		Name:                  &dst.Name,
		Status:                &dst.Status,
		NotifyEveryoneByEmail: &dst.NotifyEveryoneByEmail,
		TrendAggFunc:          &dst.TrendAggFunc,
		TrendSensitivity:      &dst.TrendSensitivity,
		TeamIDs:               &dst.TeamIDs,
		ChannelIDs:            &dst.ChannelIDs,
	})

	params, err := decodeErrorMonitorParams(mon.Params)
	if err != nil {
		diags.AddError("decode error monitor params failed", err.Error())
		return diags
	}

	var priorQuery types.String
	if dst.Params != nil {
		priorQuery = dst.Params.Query
	}

	metrics := make([]monitorMetricModel, len(params.Metrics))
	for i, mm := range params.Metrics {
		metrics[i] = monitorMetricModel{
			Name:  types.StringValue(mm.Name),
			Alias: tfutil.StringFromPtr(mm.Alias),
		}
	}
	dst.Params = &errorParamsModel{
		Metrics: metrics,
		Query:   tfutil.PreferPrior(priorQuery, types.StringValue(params.Query)),
	}

	return diags
}

// decodeErrorMonitorParams round-trips the untyped params map from
// generated.Monitor into a strongly-typed ErrorMonitorParams. The response
// stores params as map[string]any because the body is not a discriminated
// oneOf server-side; this helper is the per-type bridge.
func decodeErrorMonitorParams(raw map[string]any) (*generated.ErrorMonitorParams, error) {
	buf, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("marshal params map: %w", err)
	}
	var out generated.ErrorMonitorParams
	if err := json.Unmarshal(buf, &out); err != nil {
		return nil, fmt.Errorf("unmarshal params: %w", err)
	}
	return &out, nil
}
