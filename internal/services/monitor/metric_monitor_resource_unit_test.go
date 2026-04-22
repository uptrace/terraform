package monitor

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/stretchr/testify/require"

	"github.com/uptrace/terraform/internal/generated"
)

func metricMonitorResourceTestSchema(t *testing.T) resource.SchemaResponse {
	t.Helper()

	var resp resource.SchemaResponse
	(&MetricMonitorResource{}).Schema(context.Background(), resource.SchemaRequest{}, &resp)
	require.False(t, resp.Diagnostics.HasError())

	return resp
}

func metricMonitorResourceTestState(t *testing.T, m metricMonitorModel) tfsdk.State {
	t.Helper()

	schemaResp := metricMonitorResourceTestSchema(t)
	state := tfsdk.State{Schema: schemaResp.Schema}
	diags := state.Set(context.Background(), &m)
	require.False(t, diags.HasError())

	return state
}

func metricMonitorResourceTestPlan(t *testing.T, m metricMonitorModel) tfsdk.Plan {
	t.Helper()

	schemaResp := metricMonitorResourceTestSchema(t)
	plan := tfsdk.Plan{Schema: schemaResp.Schema}
	diags := plan.Set(context.Background(), &m)
	require.False(t, diags.HasError())

	return plan
}

func validMetricMonitorModel(projectID string, monitorID types.String) metricMonitorModel {
	return metricMonitorModel{
		ID:                    monitorID,
		ProjectID:             types.StringValue(projectID),
		Name:                  types.StringValue("latency"),
		NotifyEveryoneByEmail: types.BoolValue(false),
		TrendAggFunc:          types.StringValue("avg"),
		TrendSensitivity:      types.StringValue("medium"),
		TeamIDs:               types.SetNull(types.StringType),
		ChannelIDs:            types.SetNull(types.StringType),
		Status:                types.StringNull(),
		Params: &metricParamsModel{
			Metrics: []monitorMetricModel{
				{Name: types.StringValue("http_server_duration"), Alias: types.StringValue("$http_server")},
			},
			Query: types.StringValue("avg($http_server)"),
			Detector: &detectorModel{
				Auto: &autoDetectorModel{},
			},
		},
	}
}

func TestBuildMetricMonitorRequest_autoDetector(t *testing.T) {
	m := &metricMonitorModel{
		Name:             types.StringValue("latency"),
		TrendAggFunc:     types.StringValue("avg"),
		TrendSensitivity: types.StringValue("high"),
		TeamIDs:          types.SetNull(types.StringType),
		ChannelIDs:       types.SetNull(types.StringType),
		Params: &metricParamsModel{
			Metrics: []monitorMetricModel{
				{Name: types.StringValue("http_server_duration"), Alias: types.StringValue("$http_server")},
			},
			Query:        types.StringValue("avg($http_server)"),
			Column:       &columnModel{Name: types.StringValue("value"), Unit: types.StringValue("milliseconds")},
			Resolution:   types.Float64Value(60000),
			AbsentPoints: types.StringValue("ignore"),
			Detector: &detectorModel{
				Auto: &autoDetectorModel{
					Tolerance:      types.StringValue("medium"),
					TrainingPeriod: types.Float64Value(86400000),
				},
			},
		},
	}

	req, diags := buildMetricMonitorRequest(context.Background(), m)
	require.False(t, diags.HasError(), "unexpected errors: %v", diags)
	require.Equal(t, "latency", req.Name)
	require.Equal(t, generated.MetricMonitorRequestTypeMetric, req.Type)
	require.NotNil(t, req.Params.Column)
	require.Equal(t, "value", *req.Params.Column.Name)
	require.NotNil(t, req.Params.Resolution)
	require.InDelta(t, float32(60000), *req.Params.Resolution, 0.01)
	require.NotNil(t, req.Params.AbsentPoints)
	require.Equal(t, generated.Ignore, *req.Params.AbsentPoints)
	require.Equal(t, generated.Auto, req.Params.Detector.Type)
	require.True(t, req.Params.Detector.Params.DetectorConfig_Params_OneOf.IsB())
	require.NotNil(t, req.Params.Detector.Params.DetectorConfig_Params_OneOf.B.Tolerance)
	require.Equal(t, generated.AutoDetectorParamsToleranceMedium, *req.Params.Detector.Params.DetectorConfig_Params_OneOf.B.Tolerance)
}

func TestBuildMetricMonitorRequest_manualDetector(t *testing.T) {
	m := &metricMonitorModel{
		Name:       types.StringValue("thresh"),
		TeamIDs:    types.SetNull(types.StringType),
		ChannelIDs: types.SetNull(types.StringType),
		Params: &metricParamsModel{
			Metrics: []monitorMetricModel{
				{Name: types.StringValue("m"), Alias: types.StringValue("$x")},
			},
			Query: types.StringValue("sum($x)"),
			Detector: &detectorModel{
				Manual: &manualDetectorModel{
					MinValue: types.Float64Value(1),
					MaxValue: types.Float64Value(100),
					Recovery: &recoveryModel{
						MinValue: types.Float64Value(10),
						MaxValue: types.Float64Value(90),
					},
				},
			},
		},
	}

	req, diags := buildMetricMonitorRequest(context.Background(), m)
	require.False(t, diags.HasError(), "unexpected errors: %v", diags)
	require.Equal(t, generated.Manual, req.Params.Detector.Type)
	require.True(t, req.Params.Detector.Params.DetectorConfig_Params_OneOf.IsA())
	manual := req.Params.Detector.Params.DetectorConfig_Params_OneOf.A
	require.InDelta(t, float32(1), *manual.MinValue, 0.01)
	require.InDelta(t, float32(100), *manual.MaxValue, 0.01)
	require.NotNil(t, manual.Recovery)
	require.InDelta(t, float32(10), *manual.Recovery.MinValue, 0.01)
}

func TestBuildMetricCreateBody_wrapsInEitherA(t *testing.T) {
	m := &metricMonitorModel{
		Name: types.StringValue("m"),
		Params: &metricParamsModel{
			Metrics: []monitorMetricModel{
				{Name: types.StringValue("m"), Alias: types.StringValue("$x")},
			},
			Query: types.StringValue("sum($x)"),
			Detector: &detectorModel{
				Auto: &autoDetectorModel{},
			},
		},
	}
	body, diags := buildMetricCreateBody(context.Background(), m)
	require.False(t, diags.HasError())
	require.True(t, body.CreateMonitorBody_OneOf.IsA(), "metric monitor should be Either's A variant")
}

func TestApplyMetricMonitor_autoDetector(t *testing.T) {
	mon := &generated.Monitor{
		ID:     101,
		Name:   "latency",
		Type:   generated.MonitorTypeMetric,
		Status: generated.Active,
		Params: map[string]any{
			"query":   "avg($http)",
			"metrics": []any{map[string]any{"name": "http", "alias": "$http"}},
			"column":  map[string]any{"name": "value", "unit": "milliseconds"},
			"detector": map[string]any{
				"type": "auto",
				"params": map[string]any{
					"tolerance":      "high",
					"trainingPeriod": 86400000,
				},
			},
		},
	}
	dst := &metricMonitorModel{
		Params: &metricParamsModel{
			Column: &columnModel{Name: types.StringValue("value"), Unit: types.StringValue("milliseconds")},
		},
	}

	diags := applyMetricMonitorToModel(mon, dst)
	require.False(t, diags.HasError(), "unexpected errors: %v", diags)
	require.NotNil(t, dst.Params)
	require.NotNil(t, dst.Params.Column)
	require.Equal(t, "value", dst.Params.Column.Name.ValueString())
	require.NotNil(t, dst.Params.Detector.Auto)
	require.Nil(t, dst.Params.Detector.Manual)
	require.Equal(t, "high", dst.Params.Detector.Auto.Tolerance.ValueString())
}

// When prior state exists but the user omitted optional fields from config,
// keep them null instead of surfacing server defaults into state.
func TestApplyMetricMonitor_columnPreservedAsNullWhenPriorExists(t *testing.T) {
	mon := &generated.Monitor{
		ID:     101,
		Name:   "latency",
		Type:   generated.MonitorTypeMetric,
		Status: generated.Active,
		Params: map[string]any{
			"query":         "avg($http)",
			"metrics":       []any{map[string]any{"name": "http", "alias": "$http"}},
			"column":        map[string]any{"name": "derived-by-server", "unit": "milliseconds"},
			"resolution":    60000,
			"absentPoints":  "alert",
			"numEvalPoints": 5,
			"detector": map[string]any{
				"type":   "auto",
				"params": map[string]any{},
			},
		},
	}
	dst := &metricMonitorModel{Params: &metricParamsModel{}}

	diags := applyMetricMonitorToModel(mon, dst)
	require.False(t, diags.HasError(), "unexpected errors: %v", diags)
	require.Nil(t, dst.Params.Column, "server-derived column must be dropped when user did not set one")
	require.True(t, dst.Params.Resolution.IsNull(), "server-defaulted resolution must stay null")
	require.True(t, dst.Params.AbsentPoints.IsNull(), "server-defaulted absent_points must stay null")
	require.True(t, dst.Params.NumEvalPoints.IsNull(), "server-defaulted num_eval_points must stay null")
}

func TestApplyMetricMonitor_importHydratesRemoteOptionalFields(t *testing.T) {
	mon := &generated.Monitor{
		ID:     105,
		Name:   "latency",
		Type:   generated.MonitorTypeMetric,
		Status: generated.Active,
		Params: map[string]any{
			"query":         "avg($http)",
			"metrics":       []any{map[string]any{"name": "http", "alias": "$http"}},
			"column":        map[string]any{"name": "derived-by-server", "unit": "milliseconds"},
			"resolution":    60000,
			"absentPoints":  "alert",
			"numEvalPoints": 5,
			"timeOffset":    30000,
			"detector": map[string]any{
				"type": "auto",
				"params": map[string]any{
					"tolerance": "medium",
				},
			},
		},
	}
	dst := &metricMonitorModel{}

	diags := applyMetricMonitorToModel(mon, dst)
	require.False(t, diags.HasError(), "unexpected errors: %v", diags)
	require.NotNil(t, dst.Params)
	require.NotNil(t, dst.Params.Column)
	require.Equal(t, "derived-by-server", dst.Params.Column.Name.ValueString())
	require.Equal(t, "milliseconds", dst.Params.Column.Unit.ValueString())
	require.Equal(t, 60000.0, dst.Params.Resolution.ValueFloat64())
	require.Equal(t, "alert", dst.Params.AbsentPoints.ValueString())
	require.Equal(t, int64(5), dst.Params.NumEvalPoints.ValueInt64())
	require.Equal(t, 30000.0, dst.Params.TimeOffset.ValueFloat64())
}

// User sets column.unit only. The backend auto-fills Column.Name from the
// query alias and echoes it back. State must keep name=null to match the plan.
func TestApplyMetricMonitor_columnPartialUnitOnly(t *testing.T) {
	mon := &generated.Monitor{
		ID:     103,
		Name:   "partial-col",
		Type:   generated.MonitorTypeMetric,
		Status: generated.Active,
		Params: map[string]any{
			"query":   "avg($http)",
			"metrics": []any{map[string]any{"name": "http", "alias": "$http"}},
			"column":  map[string]any{"name": "$http", "unit": "milliseconds"},
			"detector": map[string]any{
				"type":   "auto",
				"params": map[string]any{},
			},
		},
	}
	dst := &metricMonitorModel{
		Params: &metricParamsModel{
			Column: &columnModel{
				Name: types.StringNull(),
				Unit: types.StringValue("milliseconds"),
			},
		},
	}

	diags := applyMetricMonitorToModel(mon, dst)
	require.False(t, diags.HasError(), "unexpected errors: %v", diags)
	require.NotNil(t, dst.Params.Column)
	require.True(t, dst.Params.Column.Name.IsNull(), "server-derived column.name must not leak into state when user did not set one")
	require.Equal(t, "milliseconds", dst.Params.Column.Unit.ValueString())
}

// User sets column.name = "avg(x)". If the backend were to normalize it,
// state must keep the user's input form to avoid a post-apply consistency
// error. Same contract as preserveQuery.
func TestApplyMetricMonitor_columnNamePreservedOnNormalization(t *testing.T) {
	mon := &generated.Monitor{
		ID:     104,
		Name:   "norm-col",
		Type:   generated.MonitorTypeMetric,
		Status: generated.Active,
		Params: map[string]any{
			"query":   "avg($x)",
			"metrics": []any{map[string]any{"name": "m", "alias": "$x"}},
			"column":  map[string]any{"name": "avg($x)", "unit": "ms"},
			"detector": map[string]any{
				"type":   "auto",
				"params": map[string]any{},
			},
		},
	}
	dst := &metricMonitorModel{
		Params: &metricParamsModel{
			Column: &columnModel{
				Name: types.StringValue("avg(x)"),
				Unit: types.StringValue("ms"),
			},
		},
	}

	diags := applyMetricMonitorToModel(mon, dst)
	require.False(t, diags.HasError(), "unexpected errors: %v", diags)
	require.Equal(t, "avg(x)", dst.Params.Column.Name.ValueString(), "user-set column.name must be preserved verbatim")
	require.Equal(t, "ms", dst.Params.Column.Unit.ValueString())
}

func TestApplyMetricMonitor_manualDetectorWithRecovery(t *testing.T) {
	mon := &generated.Monitor{
		ID:     102,
		Name:   "thresh",
		Type:   generated.MonitorTypeMetric,
		Status: generated.Active,
		Params: map[string]any{
			"query":   "sum($x)",
			"metrics": []any{map[string]any{"name": "m", "alias": "$x"}},
			"detector": map[string]any{
				"type": "manual",
				"params": map[string]any{
					"minValue": 1,
					"maxValue": 100,
					"recovery": map[string]any{"minValue": 10, "maxValue": 90},
				},
			},
		},
	}
	dst := &metricMonitorModel{}

	diags := applyMetricMonitorToModel(mon, dst)
	require.False(t, diags.HasError(), "unexpected errors: %v", diags)
	require.NotNil(t, dst.Params.Detector.Manual)
	require.Equal(t, float64(1), dst.Params.Detector.Manual.MinValue.ValueFloat64())
	require.NotNil(t, dst.Params.Detector.Manual.Recovery)
	require.Equal(t, float64(10), dst.Params.Detector.Manual.Recovery.MinValue.ValueFloat64())
}

func TestApplyMetricMonitor_rejectsTypeMismatch(t *testing.T) {
	mon := &generated.Monitor{
		ID:     77,
		Name:   "not-a-metric",
		Type:   generated.MonitorTypeError,
		Status: generated.Active,
	}
	dst := &metricMonitorModel{}
	diags := applyMetricMonitorToModel(mon, dst)
	require.True(t, diags.HasError(), "expected error when API type does not match resource type")
}

func TestValidateDetector(t *testing.T) {
	t.Run("both manual and auto set", func(t *testing.T) {
		m := &metricParamsModel{
			Detector: &detectorModel{
				Manual: &manualDetectorModel{},
				Auto:   &autoDetectorModel{},
			},
		}
		var diags diag.Diagnostics
		validateDetector(m, &diags)
		require.True(t, diags.HasError())
	})
	t.Run("neither set", func(t *testing.T) {
		m := &metricParamsModel{Detector: &detectorModel{}}
		var diags diag.Diagnostics
		validateDetector(m, &diags)
		require.True(t, diags.HasError())
	})
	t.Run("auto only", func(t *testing.T) {
		m := &metricParamsModel{
			Detector: &detectorModel{Auto: &autoDetectorModel{}},
		}
		var diags diag.Diagnostics
		validateDetector(m, &diags)
		require.False(t, diags.HasError())
	})
	t.Run("manual only", func(t *testing.T) {
		m := &metricParamsModel{
			Detector: &detectorModel{Manual: &manualDetectorModel{}},
		}
		var diags diag.Diagnostics
		validateDetector(m, &diags)
		require.False(t, diags.HasError())
	})
}

func TestMetricMonitorCreate_rejectsProjectIDOutsideUint32(t *testing.T) {
	ctx := context.Background()
	reqPlan := metricMonitorResourceTestPlan(t, validMetricMonitorModel("4294967296", types.StringNull()))
	req := resource.CreateRequest{Plan: reqPlan}
	resp := resource.CreateResponse{}

	(&MetricMonitorResource{}).Create(ctx, req, &resp)

	require.True(t, resp.Diagnostics.HasError())
	require.Contains(t, resp.Diagnostics.Errors()[0].Summary(), "invalid project_id")
}

func TestMetricMonitorRead_rejectsProjectIDOutsideUint32(t *testing.T) {
	ctx := context.Background()
	reqState := metricMonitorResourceTestState(t, validMetricMonitorModel("4294967296", types.StringValue("1")))
	req := resource.ReadRequest{State: reqState}
	resp := resource.ReadResponse{State: reqState}

	(&MetricMonitorResource{}).Read(ctx, req, &resp)

	require.True(t, resp.Diagnostics.HasError())
	require.Contains(t, resp.Diagnostics.Errors()[0].Summary(), "invalid project_id")
}

func TestMetricMonitorUpdate_rejectsProjectIDOutsideUint32(t *testing.T) {
	ctx := context.Background()
	model := validMetricMonitorModel("4294967296", types.StringValue("1"))
	reqState := metricMonitorResourceTestState(t, model)
	reqPlan := metricMonitorResourceTestPlan(t, model)
	req := resource.UpdateRequest{State: reqState, Plan: reqPlan}
	resp := resource.UpdateResponse{State: reqState}

	(&MetricMonitorResource{}).Update(ctx, req, &resp)

	require.True(t, resp.Diagnostics.HasError())
	require.Contains(t, resp.Diagnostics.Errors()[0].Summary(), "invalid project_id")
}

func TestMetricMonitorDelete_rejectsProjectIDOutsideUint32(t *testing.T) {
	ctx := context.Background()
	reqState := metricMonitorResourceTestState(t, validMetricMonitorModel("4294967296", types.StringValue("1")))
	req := resource.DeleteRequest{State: reqState}
	resp := resource.DeleteResponse{State: reqState}

	(&MetricMonitorResource{}).Delete(ctx, req, &resp)

	require.True(t, resp.Diagnostics.HasError())
	require.Contains(t, resp.Diagnostics.Errors()[0].Summary(), "invalid project_id")
}
