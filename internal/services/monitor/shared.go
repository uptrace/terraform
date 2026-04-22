package monitor

import (
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/uptrace/terraform/internal/generated"
	"github.com/uptrace/terraform/internal/tfutil"
)

var (
	trendAggFuncs      = []string{"sum", "avg", "median", "last"}
	trendSensitivities = []string{"low", "medium", "high"}
)

const (
	trendAggFuncDefault     = "sum"
	trendSensitivityDefault = "medium"
)

// sharedAttributes returns the schema attributes common to error and metric
// monitors. Each concrete resource appends its own `params` attribute.
func sharedAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"id": schema.StringAttribute{
			Computed:    true,
			Description: "Monitor ID.",
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.UseStateForUnknown(),
			},
		},
		"project_id": schema.StringAttribute{
			Required:    true,
			Description: "Project ID this monitor belongs to. Changing this forces recreation.",
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.RequiresReplace(),
			},
		},
		"name": schema.StringAttribute{
			Required:    true,
			Description: "Human-readable monitor name.",
		},
		"notify_everyone_by_email": schema.BoolAttribute{
			Optional:    true,
			Computed:    true,
			Default:     booldefault.StaticBool(false),
			Description: "Notify every project member by email when the monitor fires.",
		},
		"trend_agg_func": schema.StringAttribute{
			Optional:    true,
			Computed:    true,
			Default:     stringdefault.StaticString(trendAggFuncDefault),
			Description: "Aggregation function used to compute the trend baseline (sum, avg, median, last).",
			Validators: []validator.String{
				stringvalidator.OneOf(trendAggFuncs...),
			},
		},
		"trend_sensitivity": schema.StringAttribute{
			Optional:    true,
			Computed:    true,
			Default:     stringdefault.StaticString(trendSensitivityDefault),
			Description: "Sensitivity level of the trend-based anomaly detector (low, medium, high).",
			Validators: []validator.String{
				stringvalidator.OneOf(trendSensitivities...),
			},
		},
		"team_ids": schema.SetAttribute{
			Optional:    true,
			ElementType: types.StringType,
			Description: "Team IDs to notify when the monitor fires. Removing this attribute clears the association server-side.",
		},
		"channel_ids": schema.SetAttribute{
			Optional:    true,
			ElementType: types.StringType,
			Description: "Notification channel IDs from typed channel resources such as uptrace_webhook_channel.id. Removing this attribute clears the association server-side.",
		},
		"status": schema.StringAttribute{
			Computed:    true,
			Description: "Runtime status (active, paused, firing, no_data, disabled).",
		},
	}
}

// monitorMetricModel maps a single entry of the metrics list that both the
// error- and metric-monitor `params` blocks expose.
type monitorMetricModel struct {
	Name  types.String `tfsdk:"name"`
	Alias types.String `tfsdk:"alias"`
}

// monitorMetricsAttribute is the schema for the `metrics` list inside each
// `params` block.
func monitorMetricsAttribute() schema.Attribute {
	return schema.ListNestedAttribute{
		Required:    true,
		Description: "Metrics referenced by the query. At least one metric must be provided.",
		Validators: []validator.List{
			listvalidator.SizeAtLeast(1),
		},
		NestedObject: schema.NestedAttributeObject{
			Attributes: map[string]schema.Attribute{
				"name": schema.StringAttribute{
					Required:    true,
					Description: "Metric name.",
				},
				"alias": schema.StringAttribute{
					Optional:    true,
					Description: "Alias used in the query expression.",
				},
			},
		},
	}
}

// sharedFields is a pointer bundle that each concrete-resource apply uses to
// write shared fields from the API response into its flat model. Keeping it a
// parameter pack (rather than an embedded struct) avoids tfsdk reflection
// issues with non-flat models.
type sharedFields struct {
	ID                    *types.String
	Name                  *types.String
	Status                *types.String
	NotifyEveryoneByEmail *types.Bool
	TrendAggFunc          *types.String
	TrendSensitivity      *types.String
	TeamIDs               *types.Set
	ChannelIDs            *types.Set
}

// applyShared writes monitor fields common to both types from mon into f.
func applyShared(mon *generated.Monitor, f sharedFields) {
	*f.ID = types.StringValue(strconv.FormatInt(mon.ID, 10))
	*f.Name = types.StringValue(mon.Name)
	*f.Status = types.StringValue(string(mon.Status))
	if mon.NotifyEveryoneByEmail != nil {
		*f.NotifyEveryoneByEmail = types.BoolValue(*mon.NotifyEveryoneByEmail)
	} else {
		*f.NotifyEveryoneByEmail = types.BoolValue(false)
	}
	*f.TrendAggFunc = tfutil.EnumToValueOrDefault(mon.TrendAggFunc, trendAggFuncDefault)
	*f.TrendSensitivity = tfutil.EnumToValueOrDefault(mon.TrendSensitivity, trendSensitivityDefault)
	*f.TeamIDs = tfutil.IntSetFromSlice(mon.TeamIds, *f.TeamIDs)
	*f.ChannelIDs = tfutil.IntSetFromSlice(mon.ChannelIds, *f.ChannelIDs)
}
