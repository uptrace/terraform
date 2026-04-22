package notifchan

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/stretchr/testify/require"

	"github.com/uptrace/terraform/internal/generated"
)

type testChannelModel struct {
	projectID types.String
	id        types.String
	name      types.String
}

// --- applyShared ---

func TestApplyShared_monitorIDsExplicitEmptyPreserved(t *testing.T) {
	ch := &generated.NotificationChannel{
		ID: 42, Name: "c", Type: generated.Slack, Status: generated.Delivering,
		MonitorIds: nil,
	}
	priorEmpty, _ := types.SetValueFrom(context.Background(), types.StringType, []string{})
	var (
		id, name, status, condition types.String
		matchAll                    types.Bool
		priorities                  types.List
		monitorIDs                  = priorEmpty
	)
	diags := applyShared(context.Background(), ch, sharedFields{
		ID: &id, Name: &name, Status: &status, MatchAll: &matchAll,
		Condition: &condition, Priorities: &priorities, MonitorIDs: &monitorIDs,
	})
	require.False(t, diags.HasError(), "unexpected errors: %v", diags)
	require.False(t, monitorIDs.IsNull(), "explicit empty set must round-trip")
	require.Equal(t, 0, len(monitorIDs.Elements()))
}

func TestApplyShared_monitorIDsNilBecomesNull(t *testing.T) {
	ch := &generated.NotificationChannel{
		ID: 42, Name: "c", Type: generated.Slack, Status: generated.Delivering,
	}
	var (
		id, name, status, condition types.String
		matchAll                    types.Bool
		priorities                  types.List
		monitorIDs                  = types.SetNull(types.StringType)
	)
	diags := applyShared(context.Background(), ch, sharedFields{
		ID: &id, Name: &name, Status: &status, MatchAll: &matchAll,
		Condition: &condition, Priorities: &priorities, MonitorIDs: &monitorIDs,
	})
	require.False(t, diags.HasError())
	require.True(t, monitorIDs.IsNull(), "nil prior + empty API → SetNull")
}

func TestApplyShared_conditionNilBecomesNull(t *testing.T) {
	ch := &generated.NotificationChannel{
		ID: 42, Name: "c", Type: generated.Slack, Status: generated.Delivering,
		Condition: nil,
	}
	var (
		id, name, status, condition types.String
		matchAll                    types.Bool
		priorities                  types.List
		monitorIDs                  = types.SetNull(types.StringType)
	)
	diags := applyShared(context.Background(), ch, sharedFields{
		ID: &id, Name: &name, Status: &status, MatchAll: &matchAll,
		Condition: &condition, Priorities: &priorities, MonitorIDs: &monitorIDs,
	})
	require.False(t, diags.HasError())
	require.True(t, condition.IsNull())
}

func TestApplyShared_matchAllDefaultsToTrueWhenNil(t *testing.T) {
	ch := &generated.NotificationChannel{
		ID: 42, Name: "c", Type: generated.Slack, Status: generated.Delivering,
		MatchAll: nil,
	}
	var (
		id, name, status, condition types.String
		matchAll                    types.Bool
		priorities                  types.List
		monitorIDs                  = types.SetNull(types.StringType)
	)
	diags := applyShared(context.Background(), ch, sharedFields{
		ID: &id, Name: &name, Status: &status, MatchAll: &matchAll,
		Condition: &condition, Priorities: &priorities, MonitorIDs: &monitorIDs,
	})
	require.False(t, diags.HasError())
	require.True(t, matchAll.ValueBool(), "nil match_all must default to true")
}

// --- buildSharedRequest ---

func TestBuildSharedRequest_monitorIDsInvalidIDEmitsDiagnostic(t *testing.T) {
	badIDs, _ := types.SetValueFrom(context.Background(), types.StringType, []string{"12", "not-a-number"})
	priorities, _ := types.ListValueFrom(context.Background(), types.StringType, []string{"high"})
	name := types.StringValue("x")
	matchAll := types.BoolValue(true)
	condition := types.StringNull()

	body := &generated.NotificationChannelRequest{}
	diags := buildSharedRequest(context.Background(), sharedFields{
		Name: &name, MatchAll: &matchAll, Condition: &condition,
		Priorities: &priorities, MonitorIDs: &badIDs,
	}, body)
	require.True(t, diags.HasError(), "non-numeric monitor_id must emit a diagnostic")
	require.Len(t, diags, 1)
	require.Equal(t, "invalid monitor_id", diags[0].Summary())
	attrDiag, ok := diags[0].(diag.DiagnosticWithPath)
	require.True(t, ok, "invalid monitor_id diagnostic must be attributed to monitor_ids")
	require.True(t, path.Root("monitor_ids").Equal(attrDiag.Path()))
}

func TestBuildSharedRequest_prioritiesRoundTrip(t *testing.T) {
	priorities, _ := types.ListValueFrom(context.Background(), types.StringType, []string{"high", "medium"})
	monitorIDs := types.SetNull(types.StringType)
	name := types.StringValue("x")
	matchAll := types.BoolValue(true)
	condition := types.StringNull()

	body := &generated.NotificationChannelRequest{}
	diags := buildSharedRequest(context.Background(), sharedFields{
		Name: &name, MatchAll: &matchAll, Condition: &condition,
		Priorities: &priorities, MonitorIDs: &monitorIDs,
	}, body)
	require.False(t, diags.HasError())
	require.Equal(t, []generated.NotificationChannelRequestPriorities{
		"high", "medium",
	}, body.Priorities)
}

// --- validateMatchAllRule ---

func TestValidateMatchAllRule(t *testing.T) {
	emptySet, _ := types.SetValueFrom(context.Background(), types.StringType, []string{})
	nonEmpty, _ := types.SetValueFrom(context.Background(), types.StringType, []string{"1"})

	tests := []struct {
		name       string
		matchAll   types.Bool
		monitorIDs types.Set
		wantError  bool
	}{
		{"match_all=true never errors", types.BoolValue(true), types.SetNull(types.StringType), false},
		{"match_all=false + null ids errors", types.BoolValue(false), types.SetNull(types.StringType), true},
		{"match_all=false + empty ids errors", types.BoolValue(false), emptySet, true},
		{"match_all=false + non-empty ids ok", types.BoolValue(false), nonEmpty, false},
		{"match_all=null never errors", types.BoolNull(), types.SetNull(types.StringType), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var diags diag.Diagnostics
			validateMatchAllRule(tc.matchAll, tc.monitorIDs, &diags)
			require.Equal(t, tc.wantError, diags.HasError())
		})
	}
}

// --- valueStringToPtr ---

func TestValueStringToPtr(t *testing.T) {
	require.Nil(t, valueStringToPtr(types.StringNull()))
	require.Nil(t, valueStringToPtr(types.StringUnknown()))
	v := valueStringToPtr(types.StringValue("x"))
	require.NotNil(t, v)
	require.Equal(t, "x", *v)
}

// --- rejectChannelType ---

func TestRejectChannelType_mismatch(t *testing.T) {
	ch := &generated.NotificationChannel{ID: 1, Type: generated.Slack}
	var diags diag.Diagnostics
	rejectChannelType(generated.NotificationChannelTypeWebhook, ch, &diags)
	require.True(t, diags.HasError())
}

func TestRejectChannelType_match(t *testing.T) {
	ch := &generated.NotificationChannel{ID: 1, Type: generated.Slack}
	var diags diag.Diagnostics
	rejectChannelType(generated.Slack, ch, &diags)
	require.False(t, diags.HasError())
}

func testChannelCRUD(t *testing.T) channelCRUD[testChannelModel] {
	t.Helper()

	return channelCRUD[testChannelModel]{
		TypeName: "test_channel",
		ProjectID: func(m *testChannelModel) types.String {
			return m.projectID
		},
		ID: func(m *testChannelModel) types.String {
			return m.id
		},
		Name: func(m *testChannelModel) types.String {
			return m.name
		},
		Build: func(context.Context, *testChannelModel) (*generated.NotificationChannelRequest, diag.Diagnostics) {
			t.Fatalf("Build should not be called when project_id parsing fails")
			return nil, nil
		},
		Apply: func(context.Context, *generated.NotificationChannel, *testChannelModel) diag.Diagnostics {
			t.Fatalf("Apply should not be called when project_id parsing fails")
			return nil
		},
	}
}

func TestChannelCreate_rejectsProjectIDOutsideUint32(t *testing.T) {
	diags := channelCreate(context.Background(), nil, &testChannelModel{
		projectID: types.StringValue("4294967296"),
		name:      types.StringValue("test"),
	}, testChannelCRUD(t))

	require.True(t, diags.HasError())
	require.Contains(t, diags.Errors()[0].Summary(), "invalid project_id")
}

func TestChannelRead_rejectsProjectIDOutsideUint32(t *testing.T) {
	removeResource, diags := channelRead(context.Background(), nil, &testChannelModel{
		projectID: types.StringValue("4294967296"),
		id:        types.StringValue("1"),
		name:      types.StringValue("test"),
	}, testChannelCRUD(t))

	require.False(t, removeResource)
	require.True(t, diags.HasError())
	require.Contains(t, diags.Errors()[0].Summary(), "invalid project_id")
}

func TestChannelUpdate_rejectsProjectIDOutsideUint32(t *testing.T) {
	diags := channelUpdate(context.Background(), nil, &testChannelModel{
		projectID: types.StringValue("4294967296"),
		id:        types.StringValue("1"),
		name:      types.StringValue("test"),
	}, testChannelCRUD(t))

	require.True(t, diags.HasError())
	require.Contains(t, diags.Errors()[0].Summary(), "invalid project_id")
}

func TestChannelDelete_rejectsProjectIDOutsideUint32(t *testing.T) {
	diags := channelDelete(context.Background(), nil, &testChannelModel{
		projectID: types.StringValue("4294967296"),
		id:        types.StringValue("1"),
		name:      types.StringValue("test"),
	}, testChannelCRUD(t))

	require.True(t, diags.HasError())
	require.Contains(t, diags.Errors()[0].Summary(), "invalid project_id")
}
