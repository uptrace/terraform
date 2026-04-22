package monitor

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/uptrace/oapi-codegen-dd/v3/pkg/runtime"

	"github.com/uptrace/terraform/internal/generated"
	"github.com/uptrace/terraform/internal/tfutil"
)

var (
	absentPointsKinds = []string{"ignore", "alert", "zero"}
	tolerances        = []string{"low", "medium", "high"}
)

type metricParamsModel struct {
	Metrics       []monitorMetricModel `tfsdk:"metrics"`
	Query         types.String         `tfsdk:"query"`
	Column        *columnModel         `tfsdk:"column"`
	Resolution    types.Float64        `tfsdk:"resolution"`
	NumEvalPoints types.Int64          `tfsdk:"num_eval_points"`
	AbsentPoints  types.String         `tfsdk:"absent_points"`
	TimeOffset    types.Float64        `tfsdk:"time_offset"`
	Detector      *detectorModel       `tfsdk:"detector"`
}

type columnModel struct {
	Name types.String `tfsdk:"name"`
	Unit types.String `tfsdk:"unit"`
}

type detectorModel struct {
	Manual *manualDetectorModel `tfsdk:"manual"`
	Auto   *autoDetectorModel   `tfsdk:"auto"`
}

type manualDetectorModel struct {
	MinValue types.Float64  `tfsdk:"min_value"`
	MaxValue types.Float64  `tfsdk:"max_value"`
	Recovery *recoveryModel `tfsdk:"recovery"`
}

type recoveryModel struct {
	MinValue types.Float64 `tfsdk:"min_value"`
	MaxValue types.Float64 `tfsdk:"max_value"`
}

type autoDetectorModel struct {
	Tolerance      types.String  `tfsdk:"tolerance"`
	TrainingPeriod types.Float64 `tfsdk:"training_period"`
	MinDevFraction types.Float64 `tfsdk:"min_dev_fraction"`
	MinDevAbsolute types.Float64 `tfsdk:"min_dev_absolute"`
}

func metricParamsAttribute() schema.Attribute {
	return schema.SingleNestedAttribute{
		Required:    true,
		Description: "Metric monitor query parameters.",
		Attributes: map[string]schema.Attribute{
			"metrics": monitorMetricsAttribute(),
			"query": schema.StringAttribute{
				Required:    true,
				Description: "MQL query expression.",
			},
			"column": schema.SingleNestedAttribute{
				Optional:    true,
				Description: "Query result column that the detector evaluates. When unset, the server derives one from the query and the provider preserves the null state.",
				Attributes: map[string]schema.Attribute{
					"name": schema.StringAttribute{
						Optional:    true,
						Description: "Name of the query result column being monitored.",
					},
					"unit": schema.StringAttribute{
						Optional:    true,
						Description: "Display unit for the column values (e.g. \"milliseconds\", \"bytes\").",
					},
				},
			},
			"resolution": schema.Float64Attribute{
				Optional:    true,
				Description: "Evaluation resolution in milliseconds.",
			},
			"num_eval_points": schema.Int64Attribute{
				Optional:    true,
				Description: "Number of consecutive evaluation points that must breach the threshold.",
			},
			"absent_points": schema.StringAttribute{
				Optional:    true,
				Description: "How to treat gaps in the query output (ignore, alert, zero).",
				Validators: []validator.String{
					stringvalidator.OneOf(absentPointsKinds...),
				},
			},
			"time_offset": schema.Float64Attribute{
				Optional:    true,
				Description: "Time offset in milliseconds applied to the query before evaluation.",
			},
			"detector": schema.SingleNestedAttribute{
				Required:    true,
				Description: "Detector configuration. Exactly one of `manual` or `auto` must be set.",
				Attributes: map[string]schema.Attribute{
					"manual": schema.SingleNestedAttribute{
						Optional:    true,
						Description: "Manual threshold detector.",
						Attributes: map[string]schema.Attribute{
							"min_value": schema.Float64Attribute{Optional: true, Description: "Alert when value falls below this threshold."},
							"max_value": schema.Float64Attribute{Optional: true, Description: "Alert when value rises above this threshold."},
							"recovery": schema.SingleNestedAttribute{
								Optional:    true,
								Description: "Hysteresis thresholds used to clear an active alert.",
								Attributes: map[string]schema.Attribute{
									"min_value": schema.Float64Attribute{Optional: true, Description: "Clear the alert once the value rises back above this threshold."},
									"max_value": schema.Float64Attribute{Optional: true, Description: "Clear the alert once the value falls back below this threshold."},
								},
							},
						},
					},
					"auto": schema.SingleNestedAttribute{
						Optional:    true,
						Description: "Automatic trend-based detector.",
						Attributes: map[string]schema.Attribute{
							"tolerance": schema.StringAttribute{
								Optional:    true,
								Description: "Deviation tolerance (low, medium, high).",
								Validators: []validator.String{
									stringvalidator.OneOf(tolerances...),
								},
							},
							"training_period":  schema.Float64Attribute{Optional: true, Description: "Training period in milliseconds."},
							"min_dev_fraction": schema.Float64Attribute{Optional: true, Description: "Minimum deviation as a fraction of the baseline."},
							"min_dev_absolute": schema.Float64Attribute{Optional: true, Description: "Minimum absolute deviation from the baseline."},
						},
					},
				},
			},
		},
	}
}

func buildMetricCreateBody(ctx context.Context, m *metricMonitorModel) (*generated.CreateMonitorBody, diag.Diagnostics) {
	req, diags := buildMetricMonitorRequest(ctx, m)
	if diags.HasError() {
		return nil, diags
	}
	return &generated.CreateMonitorBody{
		CreateMonitorBody_OneOf: &generated.CreateMonitorBody_OneOf{
			Either: runtime.NewEitherFromA[generated.MetricMonitorRequest, generated.ErrorMonitorRequest](*req),
		},
	}, diags
}

func buildMetricUpdateBody(ctx context.Context, m *metricMonitorModel) (*generated.UpdateMonitorBody, diag.Diagnostics) {
	req, diags := buildMetricMonitorRequest(ctx, m)
	if diags.HasError() {
		return nil, diags
	}
	return &generated.UpdateMonitorBody{
		UpdateMonitorBody_OneOf: &generated.UpdateMonitorBody_OneOf{
			Either: runtime.NewEitherFromA[generated.MetricMonitorRequest, generated.ErrorMonitorRequest](*req),
		},
	}, diags
}

func buildMetricMonitorRequest(ctx context.Context, m *metricMonitorModel) (*generated.MetricMonitorRequest, diag.Diagnostics) {
	var diags diag.Diagnostics

	req := &generated.MetricMonitorRequest{
		Name: m.Name.ValueString(),
		Type: generated.MetricMonitorRequestTypeMetric,
	}

	if !m.NotifyEveryoneByEmail.IsNull() && !m.NotifyEveryoneByEmail.IsUnknown() {
		v := m.NotifyEveryoneByEmail.ValueBool()
		req.NotifyEveryoneByEmail = &v
	}
	if !m.TrendAggFunc.IsNull() && !m.TrendAggFunc.IsUnknown() {
		v := generated.MetricMonitorRequestTrendAggFunc(m.TrendAggFunc.ValueString())
		req.TrendAggFunc = &v
	}
	if !m.TrendSensitivity.IsNull() && !m.TrendSensitivity.IsUnknown() {
		v := generated.MetricMonitorRequestTrendSensitivity(m.TrendSensitivity.ValueString())
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
	params, d := buildMetricParams(m.Params)
	diags.Append(d...)
	if d.HasError() {
		return nil, diags
	}
	req.Params = *params

	return req, diags
}

func buildMetricParams(m *metricParamsModel) (*generated.MetricMonitorParams, diag.Diagnostics) {
	var diags diag.Diagnostics

	metrics := make([]generated.MonitorMetric, len(m.Metrics))
	for i, mm := range m.Metrics {
		mem := generated.MonitorMetric{Name: mm.Name.ValueString()}
		if !mm.Alias.IsNull() && !mm.Alias.IsUnknown() {
			v := mm.Alias.ValueString()
			mem.Alias = &v
		}
		metrics[i] = mem
	}

	params := &generated.MetricMonitorParams{
		Query:   m.Query.ValueString(),
		Metrics: metrics,
	}

	if m.Column != nil {
		col := &generated.Column{}
		if !m.Column.Name.IsNull() && !m.Column.Name.IsUnknown() {
			v := m.Column.Name.ValueString()
			col.Name = &v
		}
		if !m.Column.Unit.IsNull() && !m.Column.Unit.IsUnknown() {
			v := m.Column.Unit.ValueString()
			col.Unit = &v
		}
		params.Column = col
	}
	if !m.Resolution.IsNull() && !m.Resolution.IsUnknown() {
		v := float32(m.Resolution.ValueFloat64())
		params.Resolution = &v
	}
	if !m.NumEvalPoints.IsNull() && !m.NumEvalPoints.IsUnknown() {
		v := int(m.NumEvalPoints.ValueInt64())
		params.NumEvalPoints = &v
	}
	if !m.AbsentPoints.IsNull() && !m.AbsentPoints.IsUnknown() {
		v := generated.MetricMonitorParamsAbsentPoints(m.AbsentPoints.ValueString())
		params.AbsentPoints = &v
	}
	if !m.TimeOffset.IsNull() && !m.TimeOffset.IsUnknown() {
		v := float32(m.TimeOffset.ValueFloat64())
		params.TimeOffset = &v
	}

	if m.Detector == nil {
		diags.AddAttributeError(
			path.Root("params").AtName("detector"),
			"detector is required",
			"params.detector must be set for a metric monitor.",
		)
		return nil, diags
	}
	detector, d := buildDetector(m.Detector)
	diags.Append(d...)
	if d.HasError() {
		return nil, diags
	}
	params.Detector = *detector

	return params, diags
}

func buildDetector(m *detectorModel) (*generated.DetectorConfig, diag.Diagnostics) {
	var diags diag.Diagnostics

	switch {
	case m.Manual != nil && m.Auto != nil:
		diags.AddAttributeError(
			path.Root("params").AtName("detector"),
			"exactly one detector kind required",
			"detector must set exactly one of manual or auto, not both.",
		)
		return nil, diags
	case m.Manual != nil:
		manual := buildManualDetector(m.Manual)
		oneOf := &generated.DetectorConfig_Params_OneOf{
			Either: runtime.NewEitherFromA[generated.ManualDetectorParams, generated.AutoDetectorParams](manual),
		}
		return &generated.DetectorConfig{
			Type:   generated.Manual,
			Params: generated.DetectorConfig_Params{DetectorConfig_Params_OneOf: oneOf},
		}, diags
	case m.Auto != nil:
		auto := buildAutoDetector(m.Auto)
		oneOf := &generated.DetectorConfig_Params_OneOf{
			Either: runtime.NewEitherFromB[generated.ManualDetectorParams, generated.AutoDetectorParams](auto),
		}
		return &generated.DetectorConfig{
			Type:   generated.Auto,
			Params: generated.DetectorConfig_Params{DetectorConfig_Params_OneOf: oneOf},
		}, diags
	default:
		diags.AddAttributeError(
			path.Root("params").AtName("detector"),
			"detector kind missing",
			"detector must set exactly one of manual or auto.",
		)
		return nil, diags
	}
}

func buildManualDetector(m *manualDetectorModel) generated.ManualDetectorParams {
	out := generated.ManualDetectorParams{}
	if !m.MinValue.IsNull() && !m.MinValue.IsUnknown() {
		v := float32(m.MinValue.ValueFloat64())
		out.MinValue = &v
	}
	if !m.MaxValue.IsNull() && !m.MaxValue.IsUnknown() {
		v := float32(m.MaxValue.ValueFloat64())
		out.MaxValue = &v
	}
	if m.Recovery != nil {
		rec := &generated.Recovery{}
		if !m.Recovery.MinValue.IsNull() && !m.Recovery.MinValue.IsUnknown() {
			v := float32(m.Recovery.MinValue.ValueFloat64())
			rec.MinValue = &v
		}
		if !m.Recovery.MaxValue.IsNull() && !m.Recovery.MaxValue.IsUnknown() {
			v := float32(m.Recovery.MaxValue.ValueFloat64())
			rec.MaxValue = &v
		}
		out.Recovery = rec
	}
	return out
}

func buildAutoDetector(m *autoDetectorModel) generated.AutoDetectorParams {
	out := generated.AutoDetectorParams{}
	if !m.Tolerance.IsNull() && !m.Tolerance.IsUnknown() {
		v := generated.AutoDetectorParamsTolerance(m.Tolerance.ValueString())
		out.Tolerance = &v
	}
	if !m.TrainingPeriod.IsNull() && !m.TrainingPeriod.IsUnknown() {
		v := float32(m.TrainingPeriod.ValueFloat64())
		out.TrainingPeriod = &v
	}
	if !m.MinDevFraction.IsNull() && !m.MinDevFraction.IsUnknown() {
		v := float32(m.MinDevFraction.ValueFloat64())
		out.MinDevFraction = &v
	}
	if !m.MinDevAbsolute.IsNull() && !m.MinDevAbsolute.IsUnknown() {
		v := float32(m.MinDevAbsolute.ValueFloat64())
		out.MinDevAbsolute = &v
	}
	return out
}

func applyMetricMonitorToModel(mon *generated.Monitor, dst *metricMonitorModel) diag.Diagnostics {
	var diags diag.Diagnostics

	if mon.Type != generated.MonitorTypeMetric {
		diags.AddError(
			"monitor type mismatch",
			fmt.Sprintf("expected metric monitor, API returned %q for monitor id=%d", mon.Type, mon.ID),
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

	params, err := decodeMetricMonitorParams(mon.Params)
	if err != nil {
		diags.AddError("decode metric monitor params failed", err.Error())
		return diags
	}

	prior := dst.Params

	metrics := make([]monitorMetricModel, len(params.Metrics))
	for i, mm := range params.Metrics {
		metrics[i] = monitorMetricModel{
			Name:  types.StringValue(mm.Name),
			Alias: tfutil.StringFromPtr(mm.Alias),
		}
	}

	out := &metricParamsModel{
		Metrics: metrics,
		Query:   tfutil.PreferPrior(priorMetricQuery(prior), types.StringValue(params.Query)),
	}

	importing := prior == nil

	// Optional metric-param scalars default on the server side (e.g. the
	// backend populates column from the query, resolution=60000ms, and
	// absent_points="alert" when the user leaves them unset). Preserving
	// the prior null value keeps state aligned with config and avoids a
	// post-apply consistency error; a user-set value still round-trips
	// because prior is non-null. Imports have no prior state, so in that case
	// hydrate the API values into state verbatim.
	out.AbsentPoints = tfutil.PreserveOptional(
		importing || (prior != nil && !prior.AbsentPoints.IsNull()),
		tfutil.EnumToValue(params.AbsentPoints), types.StringNull())
	out.Resolution = tfutil.PreserveOptional(
		importing || (prior != nil && !prior.Resolution.IsNull()),
		tfutil.Float32PtrToFloat64(params.Resolution), types.Float64Null())
	out.TimeOffset = tfutil.PreserveOptional(
		importing || (prior != nil && !prior.TimeOffset.IsNull()),
		tfutil.Float32PtrToFloat64(params.TimeOffset), types.Float64Null())
	out.NumEvalPoints = tfutil.PreserveOptional(
		importing || (prior != nil && !prior.NumEvalPoints.IsNull()),
		tfutil.IntPtrToInt64(params.NumEvalPoints), types.Int64Null())

	// Column is a nested object, not a scalar — preserve each inner field
	// only when prior had that field set. The backend may normalize
	// column.name/column.unit; keep the user's form via PreferPrior-with-
	// null-fallback.
	if params.Column != nil {
		switch {
		case importing:
			out.Column = &columnModel{
				Name: tfutil.StringFromPtr(params.Column.Name),
				Unit: tfutil.StringFromPtr(params.Column.Unit),
			}
		case prior != nil && prior.Column != nil:
			out.Column = &columnModel{
				Name: tfutil.PreferPrior(prior.Column.Name, types.StringNull()),
				Unit: tfutil.PreferPrior(prior.Column.Unit, types.StringNull()),
			}
		}
	}

	detector, d := detectorFromRaw(mon.Params)
	diags.Append(d...)
	if d.HasError() {
		return diags
	}
	out.Detector = detector

	dst.Params = out

	return diags
}

func priorMetricQuery(prior *metricParamsModel) types.String {
	if prior == nil {
		return types.StringNull()
	}
	return prior.Query
}

// detectorFromRaw decodes the detector subtree from the monitor's raw params
// map. The generated DetectorConfig uses an Either that guesses the variant
// by field shape, which is unreliable when Manual and Auto both happen to
// validate (e.g. empty auto vs empty manual). Driving the decode off the
// raw `detector.type` string avoids that ambiguity.
func detectorFromRaw(rawParams map[string]any) (*detectorModel, diag.Diagnostics) {
	var diags diag.Diagnostics

	rawDetector, ok := rawParams["detector"].(map[string]any)
	if !ok {
		diags.AddError("decode detector failed", "monitor params do not contain a detector object")
		return nil, diags
	}
	detectorType, _ := rawDetector["type"].(string)
	rawInner, _ := rawDetector["params"].(map[string]any)

	innerBytes, err := json.Marshal(rawInner)
	if err != nil {
		diags.AddError("encode detector params failed", err.Error())
		return nil, diags
	}

	out := &detectorModel{}
	switch detectorType {
	case string(generated.Manual):
		var manual generated.ManualDetectorParams
		if err := json.Unmarshal(innerBytes, &manual); err != nil {
			diags.AddError("decode manual detector failed", err.Error())
			return nil, diags
		}
		mm := &manualDetectorModel{
			MinValue: tfutil.Float32PtrToFloat64(manual.MinValue),
			MaxValue: tfutil.Float32PtrToFloat64(manual.MaxValue),
		}
		if manual.Recovery != nil {
			mm.Recovery = &recoveryModel{
				MinValue: tfutil.Float32PtrToFloat64(manual.Recovery.MinValue),
				MaxValue: tfutil.Float32PtrToFloat64(manual.Recovery.MaxValue),
			}
		}
		out.Manual = mm
	case string(generated.Auto):
		var auto generated.AutoDetectorParams
		if err := json.Unmarshal(innerBytes, &auto); err != nil {
			diags.AddError("decode auto detector failed", err.Error())
			return nil, diags
		}
		out.Auto = &autoDetectorModel{
			Tolerance:      tfutil.EnumToValue(auto.Tolerance),
			TrainingPeriod: tfutil.Float32PtrToFloat64(auto.TrainingPeriod),
			MinDevFraction: tfutil.Float32PtrToFloat64(auto.MinDevFraction),
			MinDevAbsolute: tfutil.Float32PtrToFloat64(auto.MinDevAbsolute),
		}
	default:
		diags.AddError("unknown detector type", fmt.Sprintf("detector.type=%q is not supported", detectorType))
		return nil, diags
	}
	return out, diags
}

func decodeMetricMonitorParams(raw map[string]any) (*generated.MetricMonitorParams, error) {
	buf, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("marshal params map: %w", err)
	}
	var out generated.MetricMonitorParams
	if err := json.Unmarshal(buf, &out); err != nil {
		return nil, fmt.Errorf("unmarshal params: %w", err)
	}
	return &out, nil
}

// validateDetector surfaces a diagnostic when the detector block is missing,
// empty, or has both variants set. Used by the metric monitor's ValidateConfig.
func validateDetector(m *metricParamsModel, diags *diag.Diagnostics) {
	if m.Detector == nil {
		diags.AddAttributeError(
			path.Root("params").AtName("detector"),
			"detector is required",
			"params.detector must be set for a metric monitor.",
		)
		return
	}
	manualSet := m.Detector.Manual != nil
	autoSet := m.Detector.Auto != nil
	switch {
	case manualSet && autoSet:
		diags.AddAttributeError(
			path.Root("params").AtName("detector"),
			"detector must set exactly one kind",
			"detector must set exactly one of `manual` or `auto`, not both.",
		)
	case !manualSet && !autoSet:
		diags.AddAttributeError(
			path.Root("params").AtName("detector"),
			"detector must set exactly one kind",
			"detector must set exactly one of `manual` or `auto`.",
		)
	}
}
