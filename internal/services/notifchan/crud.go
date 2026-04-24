package notifchan

import (
	"context"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/uptrace/terraform/internal/client"
	"github.com/uptrace/terraform/internal/generated"
	"github.com/uptrace/terraform/internal/tfutil"
)

// channelCRUD bundles the per-type hooks that vary across channel resources.
// The shared CRUD helpers drive every backend call through it so each
// typed resource only provides its schema, model, builder, and mapper.
type channelCRUD[M any] struct {
	TypeName  string // e.g. "slack_channel" — only used in log/error messages
	ProjectID func(*M) types.String
	ID        func(*M) types.String
	Name      func(*M) types.String
	Build     func(context.Context, *M) (*generated.NotificationChannelRequest, diag.Diagnostics)
	Apply     func(context.Context, *generated.NotificationChannel, *M) diag.Diagnostics
}

func channelCreate[M any](ctx context.Context, c *client.Client, plan *M, h channelCRUD[M]) diag.Diagnostics {
	var diags diag.Diagnostics

	projectID64, err := strconv.ParseUint(h.ProjectID(plan).ValueString(), 10, 32)
	if err != nil {
		diags.AddError("invalid project_id", err.Error())
		return diags
	}
	projectID := uint32(projectID64)

	body, d := h.Build(ctx, plan)
	diags.Append(d...)
	if diags.HasError() {
		return diags
	}

	tflog.Info(ctx, "creating "+h.TypeName, map[string]any{
		"project_id": h.ProjectID(plan).ValueString(),
		"name":       h.Name(plan).ValueString(),
	})

	out, err := c.API.CreateNotificationChannel(ctx, &generated.CreateNotificationChannelRequestOptions{
		PathParams: &generated.CreateNotificationChannelPath{ProjectID: projectID},
		Body:       body,
	})
	if err != nil {
		tfutil.AddAPIError(&diags, "create "+h.TypeName+" failed", err)
		return diags
	}

	diags.Append(h.Apply(ctx, &out.Channel, plan)...)
	return diags
}

// channelRead returns removeResource=true when the backend reports the
// resource is gone (404 / 403). The caller should remove it from state.
func channelRead[M any](ctx context.Context, c *client.Client, state *M, h channelCRUD[M]) (removeResource bool, diags diag.Diagnostics) {
	projectID64, err := strconv.ParseUint(h.ProjectID(state).ValueString(), 10, 32)
	if err != nil {
		diags.AddError("invalid project_id", err.Error())
		return false, diags
	}
	projectID := uint32(projectID64)
	channelID, err := strconv.ParseInt(h.ID(state).ValueString(), 10, 64)
	if err != nil {
		diags.AddError("invalid channel ID", err.Error())
		return false, diags
	}

	out, err := c.API.GetNotificationChannel(ctx, &generated.GetNotificationChannelRequestOptions{
		PathParams: &generated.GetNotificationChannelPath{ProjectID: projectID, ChannelID: channelID},
	})
	if err != nil {
		if client.IsNotFound(err) {
			return true, diags
		}
		tfutil.AddAPIError(&diags, "read "+h.TypeName+" failed", err)
		return false, diags
	}

	diags.Append(h.Apply(ctx, &out.Channel, state)...)
	return false, diags
}

func channelUpdate[M any](ctx context.Context, c *client.Client, plan *M, h channelCRUD[M]) diag.Diagnostics {
	var diags diag.Diagnostics

	projectID64, err := strconv.ParseUint(h.ProjectID(plan).ValueString(), 10, 32)
	if err != nil {
		diags.AddError("invalid project_id", err.Error())
		return diags
	}
	projectID := uint32(projectID64)
	channelID, err := strconv.ParseInt(h.ID(plan).ValueString(), 10, 64)
	if err != nil {
		diags.AddError("invalid channel ID", err.Error())
		return diags
	}

	body, d := h.Build(ctx, plan)
	diags.Append(d...)
	if diags.HasError() {
		return diags
	}

	tflog.Info(ctx, "updating "+h.TypeName, map[string]any{"id": h.ID(plan).ValueString()})

	out, err := c.API.UpdateNotificationChannel(ctx, &generated.UpdateNotificationChannelRequestOptions{
		PathParams: &generated.UpdateNotificationChannelPath{ProjectID: projectID, ChannelID: channelID},
		Body:       body,
	})
	if err != nil {
		tfutil.AddAPIError(&diags, "update "+h.TypeName+" failed", err)
		return diags
	}

	diags.Append(h.Apply(ctx, &out.Channel, plan)...)
	return diags
}

func channelDelete[M any](ctx context.Context, c *client.Client, state *M, h channelCRUD[M]) diag.Diagnostics {
	var diags diag.Diagnostics

	projectID64, err := strconv.ParseUint(h.ProjectID(state).ValueString(), 10, 32)
	if err != nil {
		diags.AddError("invalid project_id", err.Error())
		return diags
	}
	projectID := uint32(projectID64)
	channelID, err := strconv.ParseInt(h.ID(state).ValueString(), 10, 64)
	if err != nil {
		diags.AddError("invalid channel ID", err.Error())
		return diags
	}

	tflog.Info(ctx, "deleting "+h.TypeName, map[string]any{"id": h.ID(state).ValueString()})

	_, err = c.API.DeleteNotificationChannel(ctx, &generated.DeleteNotificationChannelRequestOptions{
		PathParams: &generated.DeleteNotificationChannelPath{ProjectID: projectID, ChannelID: channelID},
	})
	if err != nil && !client.IsNotFound(err) {
		tfutil.AddAPIError(&diags, "delete "+h.TypeName+" failed", err)
	}
	return diags
}
