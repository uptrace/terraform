package monitor

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/stretchr/testify/require"

	"github.com/uptrace/terraform/internal/generated"
)

func errorMonitorResourceTestSchema(t *testing.T) resource.SchemaResponse {
	t.Helper()

	var resp resource.SchemaResponse
	(&ErrorMonitorResource{}).Schema(context.Background(), resource.SchemaRequest{}, &resp)
	require.False(t, resp.Diagnostics.HasError())

	return resp
}

func errorMonitorResourceTestState(t *testing.T, m errorMonitorModel) tfsdk.State {
	t.Helper()

	schemaResp := errorMonitorResourceTestSchema(t)
	state := tfsdk.State{Schema: schemaResp.Schema}
	diags := state.Set(context.Background(), &m)
	require.False(t, diags.HasError())

	return state
}

func errorMonitorResourceTestPlan(t *testing.T, m errorMonitorModel) tfsdk.Plan {
	t.Helper()

	schemaResp := errorMonitorResourceTestSchema(t)
	plan := tfsdk.Plan{Schema: schemaResp.Schema}
	diags := plan.Set(context.Background(), &m)
	require.False(t, diags.HasError())

	return plan
}

func validErrorMonitorModel(projectID string, monitorID types.String) errorMonitorModel {
	return errorMonitorModel{
		ID:                    monitorID,
		ProjectID:             types.StringValue(projectID),
		Name:                  types.StringValue("err-monitor"),
		NotifyEveryoneByEmail: types.BoolValue(false),
		TrendAggFunc:          types.StringValue("sum"),
		TrendSensitivity:      types.StringValue("medium"),
		TeamIDs:               types.SetNull(types.StringType),
		ChannelIDs:            types.SetNull(types.StringType),
		Status:                types.StringNull(),
		Params: &errorParamsModel{
			Metrics: []monitorMetricModel{
				{Name: types.StringValue("uptrace_tracing_logs"), Alias: types.StringValue("$logs")},
			},
			Query: types.StringValue("sum($logs) | where true"),
		},
	}
}

func TestBuildErrorMonitorRequest_minimal(t *testing.T) {
	m := &errorMonitorModel{
		Name:                  types.StringValue("err-monitor"),
		NotifyEveryoneByEmail: types.BoolValue(false),
		TrendAggFunc:          types.StringValue("sum"),
		TrendSensitivity:      types.StringValue("medium"),
		TeamIDs:               types.SetNull(types.StringType),
		ChannelIDs:            types.SetNull(types.StringType),
		Params: &errorParamsModel{
			Metrics: []monitorMetricModel{
				{Name: types.StringValue("uptrace_tracing_logs"), Alias: types.StringValue("$logs")},
			},
			Query: types.StringValue("sum($logs) | where true"),
		},
	}

	req, diags := buildErrorMonitorRequest(context.Background(), m)
	require.False(t, diags.HasError(), "unexpected errors: %v", diags)
	require.Equal(t, "err-monitor", req.Name)
	require.Equal(t, generated.ErrorMonitorRequestTypeError, req.Type)
	require.NotNil(t, req.NotifyEveryoneByEmail)
	require.False(t, *req.NotifyEveryoneByEmail)
	require.Nil(t, req.TeamIds)
	require.Nil(t, req.ChannelIds)
	require.Equal(t, "sum($logs) | where true", req.Params.Query)
	require.Len(t, req.Params.Metrics, 1)
	require.Equal(t, "uptrace_tracing_logs", req.Params.Metrics[0].Name)
	require.Equal(t, "$logs", *req.Params.Metrics[0].Alias)
}

func TestBuildErrorMonitorRequest_withIDs(t *testing.T) {
	teamIDs, _ := types.SetValueFrom(context.Background(), types.StringType, []string{"5", "3"})
	channelIDs, _ := types.SetValueFrom(context.Background(), types.StringType, []string{"11"})
	m := &errorMonitorModel{
		Name:       types.StringValue("with-ids"),
		TeamIDs:    teamIDs,
		ChannelIDs: channelIDs,
		Params: &errorParamsModel{
			Metrics: []monitorMetricModel{
				{Name: types.StringValue("metric_x"), Alias: types.StringNull()},
			},
			Query: types.StringValue("sum($x)"),
		},
	}
	req, diags := buildErrorMonitorRequest(context.Background(), m)
	require.False(t, diags.HasError(), "unexpected errors: %v", diags)
	require.ElementsMatch(t, []int{5, 3}, req.TeamIds)
	require.Equal(t, []int{11}, req.ChannelIds)
	require.Nil(t, req.Params.Metrics[0].Alias)
}

func TestBuildErrorCreateBody_wrapsInEitherB(t *testing.T) {
	m := &errorMonitorModel{
		Name: types.StringValue("wrap"),
		Params: &errorParamsModel{
			Metrics: []monitorMetricModel{
				{Name: types.StringValue("metric_x"), Alias: types.StringValue("$x")},
			},
			Query: types.StringValue("sum($x)"),
		},
	}
	body, diags := buildErrorCreateBody(context.Background(), m)
	require.False(t, diags.HasError())
	require.NotNil(t, body.CreateMonitorBody_OneOf)
	require.True(t, body.CreateMonitorBody_OneOf.IsB(), "error monitor should be Either's B variant")
}

func TestApplyErrorMonitor_preservesPriorQuery(t *testing.T) {
	mon := &generated.Monitor{
		ID:     77,
		Name:   "err",
		Type:   generated.MonitorTypeError,
		Status: generated.Active,
		Params: map[string]any{
			"query": "sum($logs{}) | where _system::str = \"log:error\"",
			"metrics": []any{
				map[string]any{"name": "uptrace_tracing_logs", "alias": "$logs"},
			},
		},
	}
	dst := &errorMonitorModel{
		Params: &errorParamsModel{
			Query: types.StringValue(`sum($logs) | where _system = "log:error"`),
		},
	}
	diags := applyErrorMonitorToModel(mon, dst)
	require.False(t, diags.HasError(), "unexpected errors: %v", diags)
	require.Equal(t, `sum($logs) | where _system = "log:error"`, dst.Params.Query.ValueString())
	require.Len(t, dst.Params.Metrics, 1)
}

func TestApplyErrorMonitor_takesAPIQueryWhenPriorNull(t *testing.T) {
	mon := &generated.Monitor{
		ID:     77,
		Name:   "err",
		Type:   generated.MonitorTypeError,
		Status: generated.Active,
		Params: map[string]any{
			"query":   "sum($logs)",
			"metrics": []any{map[string]any{"name": "uptrace_tracing_logs", "alias": "$logs"}},
		},
	}
	dst := &errorMonitorModel{}

	diags := applyErrorMonitorToModel(mon, dst)
	require.False(t, diags.HasError(), "unexpected errors: %v", diags)
	require.Equal(t, "sum($logs)", dst.Params.Query.ValueString())
}

func TestApplyErrorMonitor_idSetsMapUnordered(t *testing.T) {
	mon := &generated.Monitor{
		ID:         77,
		Name:       "err",
		Type:       generated.MonitorTypeError,
		Status:     generated.Active,
		TeamIds:    []int{9, 1, 5},
		ChannelIds: []int{3, 2},
		Params: map[string]any{
			"query":   "sum($logs)",
			"metrics": []any{map[string]any{"name": "uptrace_tracing_logs", "alias": "$logs"}},
		},
	}
	dst := &errorMonitorModel{}

	diags := applyErrorMonitorToModel(mon, dst)
	require.False(t, diags.HasError(), "unexpected errors: %v", diags)
	require.False(t, dst.TeamIDs.IsNull())
	var team []string
	dst.TeamIDs.ElementsAs(context.Background(), &team, false)
	require.ElementsMatch(t, []string{"1", "5", "9"}, team)

	var ch []string
	dst.ChannelIDs.ElementsAs(context.Background(), &ch, false)
	require.ElementsMatch(t, []string{"2", "3"}, ch)
}

func TestApplyErrorMonitor_rejectsTypeMismatch(t *testing.T) {
	mon := &generated.Monitor{
		ID:     77,
		Name:   "not-an-error",
		Type:   generated.MonitorTypeMetric,
		Status: generated.Active,
	}
	dst := &errorMonitorModel{}

	diags := applyErrorMonitorToModel(mon, dst)
	require.True(t, diags.HasError(), "expected error when API type does not match resource type")
}

func TestErrorMonitorCreate_rejectsProjectIDOutsideUint32(t *testing.T) {
	ctx := context.Background()
	reqPlan := errorMonitorResourceTestPlan(t, validErrorMonitorModel("4294967296", types.StringNull()))
	req := resource.CreateRequest{Plan: reqPlan}
	resp := resource.CreateResponse{}

	(&ErrorMonitorResource{}).Create(ctx, req, &resp)

	require.True(t, resp.Diagnostics.HasError())
	require.Contains(t, resp.Diagnostics.Errors()[0].Summary(), "invalid project_id")
}

func TestErrorMonitorRead_rejectsProjectIDOutsideUint32(t *testing.T) {
	ctx := context.Background()
	reqState := errorMonitorResourceTestState(t, validErrorMonitorModel("4294967296", types.StringValue("1")))
	req := resource.ReadRequest{State: reqState}
	resp := resource.ReadResponse{State: reqState}

	(&ErrorMonitorResource{}).Read(ctx, req, &resp)

	require.True(t, resp.Diagnostics.HasError())
	require.Contains(t, resp.Diagnostics.Errors()[0].Summary(), "invalid project_id")
}

func TestErrorMonitorUpdate_rejectsProjectIDOutsideUint32(t *testing.T) {
	ctx := context.Background()
	model := validErrorMonitorModel("4294967296", types.StringValue("1"))
	reqState := errorMonitorResourceTestState(t, model)
	reqPlan := errorMonitorResourceTestPlan(t, model)
	req := resource.UpdateRequest{State: reqState, Plan: reqPlan}
	resp := resource.UpdateResponse{State: reqState}

	(&ErrorMonitorResource{}).Update(ctx, req, &resp)

	require.True(t, resp.Diagnostics.HasError())
	require.Contains(t, resp.Diagnostics.Errors()[0].Summary(), "invalid project_id")
}

func TestErrorMonitorDelete_rejectsProjectIDOutsideUint32(t *testing.T) {
	ctx := context.Background()
	reqState := errorMonitorResourceTestState(t, validErrorMonitorModel("4294967296", types.StringValue("1")))
	req := resource.DeleteRequest{State: reqState}
	resp := resource.DeleteResponse{State: reqState}

	(&ErrorMonitorResource{}).Delete(ctx, req, &resp)

	require.True(t, resp.Diagnostics.HasError())
	require.Contains(t, resp.Diagnostics.Errors()[0].Summary(), "invalid project_id")
}
