package monitor_test

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/uptrace/terraform/internal/testutil"
)

func testAccErrorMonitorConfigBasic(orgName, projectName, monitorName string) string {
	return fmt.Sprintf(`
resource "uptrace_org" "test" {
  name = %q
}

resource "uptrace_project" "test" {
  org_id = uptrace_org.test.id
  name   = %q
}

resource "uptrace_error_monitor" "test" {
  project_id = uptrace_project.test.id
  name       = %q

  params = {
    metrics = [
      { name = "uptrace_tracing_logs", alias = "$logs" }
    ]
    query = "sum($logs) | where _system in (\"log:error\", \"log:fatal\")"
  }
}
`, orgName, projectName, monitorName)
}

func TestAccErrorMonitor_basic(t *testing.T) {
	const (
		orgName     = "acc-em-basic-org"
		projectName = "acc-em-basic-project"
	)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testutil.PreCheck(t) },
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckMonitorDestroy(t, "uptrace_error_monitor"),
		Steps: []resource.TestStep{
			{
				Config: testAccErrorMonitorConfigBasic(orgName, projectName, "acc-em-basic"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("uptrace_error_monitor.test", "id"),
					resource.TestCheckResourceAttr("uptrace_error_monitor.test", "name", "acc-em-basic"),
					resource.TestCheckResourceAttr("uptrace_error_monitor.test", "trend_agg_func", "sum"),
					resource.TestCheckResourceAttr("uptrace_error_monitor.test", "trend_sensitivity", "medium"),
					resource.TestCheckResourceAttr("uptrace_error_monitor.test", "params.metrics.#", "1"),
					resource.TestCheckResourceAttr("uptrace_error_monitor.test", "params.metrics.0.name", "uptrace_tracing_logs"),
					resource.TestCheckResourceAttr("uptrace_error_monitor.test", "params.metrics.0.alias", "$logs"),
					resource.TestCheckResourceAttrSet("uptrace_error_monitor.test", "status"),
				),
			},
			{
				Config:   testAccErrorMonitorConfigBasic(orgName, projectName, "acc-em-basic"),
				PlanOnly: true,
			},
			{
				Config: testAccErrorMonitorConfigBasic(orgName, projectName, "acc-em-basic-renamed"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("uptrace_error_monitor.test", "name", "acc-em-basic-renamed"),
				),
			},
			{
				ResourceName:      "uptrace_error_monitor.test",
				ImportState:       true,
				ImportStateVerify: true,
				// The backend normalizes MQL (e.g. $logs → $logs{}, all.type → all.type::str),
				// so the imported query can differ from the user's input form even though
				// they're semantically identical.
				ImportStateVerifyIgnore: []string{"params", "status"},
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					rs, ok := s.RootModule().Resources["uptrace_error_monitor.test"]
					if !ok {
						return "", fmt.Errorf("uptrace_error_monitor.test not found in state")
					}
					return fmt.Sprintf("%s:%s", rs.Primary.Attributes["project_id"], rs.Primary.ID), nil
				},
			},
		},
	})
}

func testAccErrorMonitorConfigWithChannels(orgName, projectName, monitorName, webhookURL, channelIDsExpr string) string {
	return fmt.Sprintf(`
resource "uptrace_org" "test" {
  name = %q
}

resource "uptrace_project" "test" {
  org_id = uptrace_org.test.id
  name   = %q
}

resource "uptrace_webhook_channel" "test" {
  project_id = uptrace_project.test.id
  name       = "acc-em-chan-webhook"
  priorities = ["high"]
  url        = %q

  lifecycle {
    ignore_changes = [monitor_ids]
  }
}

resource "uptrace_error_monitor" "test" {
  project_id  = uptrace_project.test.id
  name        = %q
  channel_ids = %s

  params = {
    metrics = [
      { name = "uptrace_tracing_logs", alias = "$logs" }
    ]
    query = "sum($logs) | where _system in (\"log:error\", \"log:fatal\")"
  }
}
`, orgName, projectName, webhookURL, monitorName, channelIDsExpr)
}

func TestAccErrorMonitor_channelIDsClearOnRemove(t *testing.T) {
	const (
		orgName     = "acc-em-chan-clear-org"
		projectName = "acc-em-chan-clear-project"
		webhookURL  = "https://example.com/hooks/alert"
	)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testutil.PreCheck(t) },
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckMonitorDestroy(t, "uptrace_error_monitor"),
		Steps: []resource.TestStep{
			{
				Config: testAccErrorMonitorConfigWithChannels(orgName, projectName, "acc-em-chan-clear", webhookURL,
					"[uptrace_webhook_channel.test.id]"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("uptrace_error_monitor.test", "channel_ids.#", "1"),
					resource.TestCheckTypeSetElemAttrPair(
						"uptrace_error_monitor.test", "channel_ids.*",
						"uptrace_webhook_channel.test", "id",
					),
				),
			},
			{
				Config: testAccErrorMonitorConfigWithChannels(orgName, projectName, "acc-em-chan-clear", webhookURL, "null"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr("uptrace_error_monitor.test", "channel_ids.#"),
				),
			},
			{
				Config:   testAccErrorMonitorConfigWithChannels(orgName, projectName, "acc-em-chan-clear", webhookURL, "null"),
				PlanOnly: true,
			},
		},
	})
}

func TestAccErrorMonitor_disappearsOutOfBand(t *testing.T) {
	const (
		orgName     = "acc-em-disappear-org"
		projectName = "acc-em-disappear-project"
	)
	config := testAccErrorMonitorConfigBasic(orgName, projectName, "acc-em-disappear")
	var monitorID, projectID string

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testutil.PreCheck(t) },
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckMonitorDestroy(t, "uptrace_error_monitor"),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("uptrace_error_monitor.test", "id"),
					testutil.CaptureAttr("uptrace_error_monitor.test", "id", &monitorID),
					testutil.CaptureAttr("uptrace_error_monitor.test", "project_id", &projectID),
				),
			},
			{
				PreConfig: func() {
					deleteMonitorOutOfBand(t, projectID, monitorID)
				},
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("uptrace_error_monitor.test", "id"),
					resource.TestCheckResourceAttr("uptrace_error_monitor.test", "name", "acc-em-disappear"),
				),
			},
		},
	})
}
